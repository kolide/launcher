// Running this in parallel on the GH workers generates a lot of false positive noise. It all smells like things
// deep inside boltdb. Since we usually rerun tests until they pass, let's just disable paralleltest and linting.
//
//nolint:paralleltest
package osquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"testing"
	"testing/quick"
	"time"

	"github.com/kolide/kit/testutil"
	"github.com/kolide/launcher/ee/agent/storage"
	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/service/mock"
	"github.com/mixer/clock"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func makeTempDB(t *testing.T) (db *bbolt.DB, cleanup func()) {
	file, err := os.CreateTemp("", "kolide_launcher_test")
	if err != nil {
		t.Fatalf("creating temp file: %s", err.Error())
	}

	db, err = bbolt.Open(file.Name(), 0600, nil)
	if err != nil {
		t.Fatalf("opening bolt DB: %s", err.Error())
	}

	return db, func() {
		db.Close()
		os.Remove(file.Name())
	}
}

func makeKnapsack(t *testing.T, db *bbolt.DB) types.Knapsack {
	m := mocks.NewKnapsack(t)
	m.On("OsquerydPath").Maybe().Return("")
	m.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	m.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	m.On("Slogger").Return(multislogger.NewNopLogger())
	m.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	return m
}

func TestNewExtensionEmptyEnrollSecret(t *testing.T) {
	m := mocks.NewKnapsack(t)
	m.On("OsquerydPath").Maybe().Return("")
	m.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	m.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	m.On("Slogger").Return(multislogger.NewNopLogger())
	m.On("ReadEnrollSecret").Maybe().Return("", errors.New("test"))

	// We should be able to make an extension despite an empty enroll secret
	e, err := NewExtension(context.TODO(), &mock.KolideService{}, m, ExtensionOpts{})
	assert.Nil(t, err)
	assert.NotNil(t, e)
}

func TestNewExtensionDatabaseError(t *testing.T) {

	file, err := os.CreateTemp("", "kolide_launcher_test")
	if err != nil {
		t.Fatalf("creating temp file: %s", err.Error())
	}

	db, _ := makeTempDB(t)
	path := db.Path()
	db.Close()

	// Open read-only DB
	db, err = bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("opening bolt DB: %s", err.Error())
	}
	defer func() {
		db.Close()
		os.Remove(file.Name())
	}()

	m := mocks.NewKnapsack(t)
	m.On("ConfigStore").Return(agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.ConfigStore.String()))
	m.On("Slogger").Return(multislogger.NewNopLogger()).Maybe()

	e, err := NewExtension(context.TODO(), &mock.KolideService{}, m, ExtensionOpts{})
	assert.NotNil(t, err)
	assert.Nil(t, e)
}

func TestGetHostIdentifier(t *testing.T) {
	db, cleanup := makeTempDB(t)
	defer cleanup()

	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), &mock.KolideService{}, k, ExtensionOpts{})
	require.Nil(t, err)

	ident, err := e.getHostIdentifier()
	require.Nil(t, err)
	assert.True(t, len(ident) > 10)
	oldIdent := ident

	ident, err = e.getHostIdentifier()
	require.Nil(t, err)
	assert.Equal(t, oldIdent, ident)

	db, cleanup = makeTempDB(t)
	defer cleanup()
	k = makeKnapsack(t, db)
	e, err = NewExtension(context.TODO(), &mock.KolideService{}, k, ExtensionOpts{})
	require.Nil(t, err)

	ident, err = e.getHostIdentifier()
	require.Nil(t, err)
	// Should get different UUID with fresh DB
	assert.NotEqual(t, oldIdent, ident)
}

func TestGetHostIdentifierCorruptedData(t *testing.T) {

	// Put bad data in the DB and ensure we can still generate a fresh UUID
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), &mock.KolideService{}, k, ExtensionOpts{})
	require.Nil(t, err)

	// Put garbage UUID in DB
	err = k.ConfigStore().Set([]byte(uuidKey), []byte("garbage_uuid"))
	require.Nil(t, err)

	ident, err := e.getHostIdentifier()
	require.Nil(t, err)
	assert.True(t, len(ident) > 10)
	oldIdent := ident

	ident, err = e.getHostIdentifier()
	require.Nil(t, err)
	assert.Equal(t, oldIdent, ident)
}

func TestExtensionEnrollTransportError(t *testing.T) {

	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return "", false, errors.New("transport")
		},
	}

	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(context.Background())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnrollSecretInvalid(t *testing.T) {

	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return "", true, nil
		},
	}
	db, cleanup := makeTempDB(t)
	k := makeKnapsack(t, db)
	defer cleanup()
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(context.Background())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnroll(t *testing.T) {

	var gotEnrollSecret string
	expectedNodeKey := "node_key"
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			gotEnrollSecret = enrollSecret
			return expectedNodeKey, false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	expectedEnrollSecret := "foo_secret"
	k.On("ReadEnrollSecret").Maybe().Return(expectedEnrollSecret, nil)

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(context.Background())
	require.Nil(t, err)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	// Should not re-enroll with stored secret
	m.RequestEnrollmentFuncInvoked = false
	key, invalid, err = e.Enroll(context.Background())
	require.Nil(t, err)
	assert.False(t, m.RequestEnrollmentFuncInvoked) // Note False here.
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	e, err = NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)
	// Still should not re-enroll (because node key stored in DB)
	key, invalid, err = e.Enroll(context.Background())
	require.Nil(t, err)
	assert.False(t, m.RequestEnrollmentFuncInvoked) // Note False here.
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	// Re-enroll for new node key
	expectedNodeKey = "new_node_key"
	e.RequireReenroll(context.Background())
	assert.Empty(t, e.NodeKey)
	key, invalid, err = e.Enroll(context.Background())
	require.Nil(t, err)
	// Now enroll func should be called again
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)
}

func TestExtensionGenerateConfigsTransportError(t *testing.T) {

	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return "", false, errors.New("transport")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	k.ConfigStore().Set([]byte(nodeKeyKey), []byte("some_node_key"))
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Nil(t, configs)
	// An error with the cache empty should be returned
	assert.NotNil(t, err)
}

func TestExtensionGenerateConfigsCaching(t *testing.T) {

	configVal := `{"foo":"bar","options":{"verbose":true}}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Equal(t, map[string]string{"config": configVal}, configs)
	assert.Nil(t, err)

	// Now have requesting the config fail, and expect to get the same
	// config anyway (through the cache).
	m.RequestConfigFuncInvoked = false
	m.RequestConfigFunc = func(ctx context.Context, nodeKey string) (string, bool, error) {
		return "", false, errors.New("foobar")
	}
	configs, err = e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Equal(t, map[string]string{"config": configVal}, configs)
	// No error because config came from the cache.
	assert.Nil(t, err)
}

func TestExtensionGenerateConfigsEnrollmentInvalid(t *testing.T) {

	expectedNodeKey := "good_node_key"
	var gotNodeKey string
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			gotNodeKey = nodeKey
			return "", true, nil
		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)
	e.NodeKey = "bad_node_key"

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Nil(t, configs)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestGenerateConfigs_CannotEnrollYet(t *testing.T) {
	s := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			// Returns node_invalid
			return "", true, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("", errors.New("test"))

	e, err := NewExtension(context.TODO(), s, k, ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(context.Background())
	assert.NotNil(t, configs)
	assert.Equal(t, map[string]string{"config": "{}"}, configs)
	assert.Nil(t, err)

	// Should have tried to request config
	assert.True(t, s.RequestConfigFuncInvoked)

	// On node invalid response, should attempt to retrieve enroll secret
	k.AssertExpectations(t)

	// Since we can't retrieve the enroll secret, we shouldn't attempt to enroll yet
	assert.False(t, s.RequestEnrollmentFuncInvoked)
}

func TestExtensionGenerateConfigs(t *testing.T) {

	configVal := `{"foo":"bar","options":{"verbose":true}}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Equal(t, map[string]string{"config": configVal}, configs)
	assert.Nil(t, err)
}

func TestExtensionWriteLogsTransportError(t *testing.T) {

	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			return "", "", false, errors.New("transport")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeSnapshot, []string{"foobar"}, true)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.NotNil(t, err)
}

func TestExtensionWriteLogsEnrollmentInvalid(t *testing.T) {

	expectedNodeKey := "good_node_key"
	var gotNodeKey string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			gotNodeKey = nodeKey
			return "", "", true, nil
		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)
	e.NodeKey = "bad_node_key"

	err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeString, []string{"foobar"}, true)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionWriteLogs(t *testing.T) {

	var gotNodeKey string
	var gotLogType logger.LogType
	var gotLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			gotNodeKey = nodeKey
			gotLogType = logType
			gotLogs = logs
			return "", "", false, nil
		},
	}

	expectedNodeKey := "node_key"
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)
	e.NodeKey = expectedNodeKey

	err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeStatus, []string{"foobar"}, true)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
	assert.Equal(t, logger.LogTypeStatus, gotLogType)
	assert.Equal(t, []string{"foobar"}, gotLogs)
}

func TestKeyConversion(t *testing.T) {

	expectedUintKeyVals := []uint64{1, 2, 64, 128, 200, 1000, 2000, 500003, 10000003, 200003005}
	byteKeys := make([][]byte, 0, len(expectedUintKeyVals))
	for _, k := range expectedUintKeyVals {
		byteKeys = append(byteKeys, byteKeyFromUint64(k))
	}

	// Assert correct sorted order of byte keys generated by key function
	require.True(t, sort.SliceIsSorted(byteKeys, func(i, j int) bool { return bytes.Compare(byteKeys[i], byteKeys[j]) <= 0 }))

	uintKeyVals := make([]uint64, 0, len(expectedUintKeyVals))
	for _, k := range byteKeys {
		uintKeyVals = append(uintKeyVals, uint64FromByteKey(k))
	}

	// Assert values are the same after roundtrip conversion
	require.Equal(t, expectedUintKeyVals, uintKeyVals)
}

func TestRandomKeyConversion(t *testing.T) {

	// Check that roundtrips for random values result in the same key
	f := func(k uint64) bool {
		return k == uint64FromByteKey(byteKeyFromUint64(k))
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestByteKeyFromUint64(t *testing.T) {

	// Assert correct sorted order of keys generated by key function
	keyVals := []uint64{1, 2, 64, 128, 200, 1000, 2000, 50000, 1000000, 2000000}
	keys := make([][]byte, 0, len(keyVals))
	for _, k := range keyVals {
		keys = append(keys, byteKeyFromUint64(k))
	}

	require.True(t, sort.SliceIsSorted(keyVals, func(i, j int) bool { return keyVals[i] < keyVals[j] }))
	assert.True(t, sort.SliceIsSorted(keys, func(i, j int) bool { return bytes.Compare(keys[i], keys[j]) <= 0 }))
}

func TestExtensionWriteBufferedLogsEmpty(t *testing.T) {

	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			t.Error("Publish logs function should not be called")
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create the status logs bucket ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("Slogger").Return(multislogger.NewNopLogger()).Maybe()
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	// No buffered logs should result in success and no remote action being
	// taken.
	err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	assert.Nil(t, err)
	assert.False(t, m.PublishLogsFuncInvoked)
}

func TestExtensionWriteBufferedLogs(t *testing.T) {

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create these buckets ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.ResultLogsStore.String())
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("Slogger").Return(multislogger.NewNopLogger()).Maybe()
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	e.LogString(context.Background(), logger.LogTypeStatus, "status foo")
	e.LogString(context.Background(), logger.LogTypeStatus, "status bar")

	e.LogString(context.Background(), logger.LogTypeString, "result foo")
	e.LogString(context.Background(), logger.LogTypeString, "result bar")

	err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	assert.Nil(t, err)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, []string{"status foo", "status bar"}, gotStatusLogs)

	err = e.writeBufferedLogsForType(logger.LogTypeString)
	assert.Nil(t, err)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, []string{"result foo", "result bar"}, gotResultLogs)

	// No more logs should be written after logs flushed
	m.PublishLogsFuncInvoked = false
	gotStatusLogs = nil
	gotResultLogs = nil
	err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	assert.Nil(t, err)
	err = e.writeBufferedLogsForType(logger.LogTypeString)
	assert.Nil(t, err)
	assert.False(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)

	e.LogString(context.Background(), logger.LogTypeStatus, "status foo")

	err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	assert.Nil(t, err)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, []string{"status foo"}, gotStatusLogs)
	assert.Nil(t, gotResultLogs)
}

func TestExtensionWriteBufferedLogsEnrollmentInvalid(t *testing.T) {
	// Test for https://github.com/kolide/launcher/issues/219 in which a
	// call to writeBufferedLogsForType with an invalid node key causes a
	// deadlock.
	const expectedNodeKey = "good_node_key"
	var gotNodeKey string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			gotNodeKey = nodeKey
			return "", "", nodeKey != expectedNodeKey, nil

		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create the status logs bucket ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	e.LogString(context.Background(), logger.LogTypeStatus, "status foo")
	e.LogString(context.Background(), logger.LogTypeStatus, "status bar")

	// long timeout is due to github actions runners IO slowness
	testutil.FatalAfterFunc(t, 4*time.Second, func() {
		err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	})
	assert.Nil(t, err)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionWriteBufferedLogsLimit(t *testing.T) {

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create the status logs bucket ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.ResultLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{
		MaxBytesPerBatch: 100,
	})
	require.Nil(t, err)

	expectedStatusLogs := []string{}
	expectedResultLogs := []string{}
	for i := 0; i < 20; i++ {
		status := fmt.Sprintf("status_%3d", i)
		expectedStatusLogs = append(expectedStatusLogs, status)
		e.LogString(context.Background(), logger.LogTypeStatus, status)

		result := fmt.Sprintf("result_%3d", i)
		expectedResultLogs = append(expectedResultLogs, result)
		e.LogString(context.Background(), logger.LogTypeString, result)
	}

	// Should write first 10 logs
	e.writeBufferedLogsForType(logger.LogTypeStatus)
	e.writeBufferedLogsForType(logger.LogTypeString)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, expectedStatusLogs[:10], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[:10], gotResultLogs)

	// Should write last 10 logs
	m.PublishLogsFuncInvoked = false
	gotStatusLogs = nil
	gotResultLogs = nil
	e.writeBufferedLogsForType(logger.LogTypeStatus)
	e.writeBufferedLogsForType(logger.LogTypeString)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, expectedStatusLogs[10:], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[10:], gotResultLogs)

	// No more logs to write
	m.PublishLogsFuncInvoked = false
	gotStatusLogs = nil
	gotResultLogs = nil
	e.writeBufferedLogsForType(logger.LogTypeStatus)
	e.writeBufferedLogsForType(logger.LogTypeString)
	assert.False(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)
}

func TestExtensionWriteBufferedLogsDropsBigLog(t *testing.T) {

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create the status logs bucket ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.ResultLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{
		MaxBytesPerBatch: 15,
	})
	require.Nil(t, err)

	startLogCount, err := e.numberOfBufferedLogs(logger.LogTypeString)
	require.NoError(t, err)
	require.Equal(t, 0, startLogCount, "start with no buffered logs")

	expectedResultLogs := []string{"res1", "res2", "res3", "res4"}
	e.LogString(context.Background(), logger.LogTypeString, "this_result_is_tooooooo_big! oh noes")
	e.LogString(context.Background(), logger.LogTypeString, "res1")
	e.LogString(context.Background(), logger.LogTypeString, "res2")
	e.LogString(context.Background(), logger.LogTypeString, "this_result_is_tooooooo_big! wow")
	e.LogString(context.Background(), logger.LogTypeString, "this_result_is_tooooooo_big! scheiße")
	e.LogString(context.Background(), logger.LogTypeString, "res3")
	e.LogString(context.Background(), logger.LogTypeString, "res4")
	e.LogString(context.Background(), logger.LogTypeString, "this_result_is_tooooooo_big! darn")

	queuedLogCount, err := e.numberOfBufferedLogs(logger.LogTypeString)
	require.NoError(t, err)
	require.Equal(t, 8, queuedLogCount, "correct number of enqueued logs")

	// Should write first 3 logs
	e.writeBufferedLogsForType(logger.LogTypeString)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, expectedResultLogs[:3], gotResultLogs)

	// Should write last log
	m.PublishLogsFuncInvoked = false
	gotResultLogs = nil
	e.writeBufferedLogsForType(logger.LogTypeString)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Equal(t, expectedResultLogs[3:], gotResultLogs)

	// No more logs to write
	m.PublishLogsFuncInvoked = false
	gotResultLogs = nil
	gotStatusLogs = nil
	e.writeBufferedLogsForType(logger.LogTypeString)
	assert.False(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, gotResultLogs)
	assert.Nil(t, gotStatusLogs)

	finalLogCount, err := e.numberOfBufferedLogs(logger.LogTypeString)
	require.NoError(t, err)
	require.Equal(t, 0, finalLogCount, "no more queued logs")
}

func TestExtensionWriteLogsLoop(t *testing.T) {
	var gotStatusLogs, gotResultLogs []string
	var funcInvokedStatus, funcInvokedResult bool
	var done = make(chan struct{})
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			defer func() { done <- struct{}{} }()

			switch logType {
			case logger.LogTypeStatus:
				funcInvokedStatus = true
				gotStatusLogs = logs
			case logger.LogTypeString:
				funcInvokedResult = true
				gotResultLogs = logs
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create the status logs bucket ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.ResultLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	mockClock := clock.NewMockClock()
	expectedLoggingInterval := 10 * time.Second
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{
		MaxBytesPerBatch: 200,
		Clock:            mockClock,
		LoggingInterval:  expectedLoggingInterval,
	})
	require.Nil(t, err)

	expectedStatusLogs := []string{}
	expectedResultLogs := []string{}
	for i := 0; i < 20; i++ {
		status := fmt.Sprintf("status_%013d", i)
		expectedStatusLogs = append(expectedStatusLogs, status)
		e.LogString(context.Background(), logger.LogTypeStatus, status)

		result := fmt.Sprintf("result_%013d", i)
		expectedResultLogs = append(expectedResultLogs, result)
		e.LogString(context.Background(), logger.LogTypeString, result)
	}

	// Should write first 10 logs
	go e.Execute()
	testutil.FatalAfterFunc(t, 1*time.Second, func() {
		// PublishLogsFunc runs twice for each run of the loop
		<-done
		<-done
	})
	assert.True(t, funcInvokedStatus)
	assert.True(t, funcInvokedResult)
	assert.Nil(t, err)
	assert.Equal(t, expectedStatusLogs[:10], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[:10], gotResultLogs)

	funcInvokedStatus = false
	funcInvokedResult = false
	gotStatusLogs = nil
	gotResultLogs = nil

	// Should write last 10 logs
	mockClock.AddTime(expectedLoggingInterval + 1)
	testutil.FatalAfterFunc(t, 1*time.Second, func() {
		// PublishLogsFunc runs twice of each run of the loop
		<-done
		<-done
	})
	assert.True(t, funcInvokedStatus)
	assert.True(t, funcInvokedResult)
	assert.Nil(t, err)
	assert.Equal(t, expectedStatusLogs[10:], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[10:], gotResultLogs)

	funcInvokedStatus = false
	funcInvokedResult = false
	gotStatusLogs = nil
	gotResultLogs = nil

	// No more logs to write
	mockClock.AddTime(expectedLoggingInterval + 1)
	// Block to ensure publish function could be called if the logic is
	// incorrect
	time.Sleep(1 * time.Millisecond)
	assert.False(t, funcInvokedStatus)
	assert.False(t, funcInvokedResult)
	assert.Nil(t, err)
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)

	testutil.FatalAfterFunc(t, 3*time.Second, func() {
		e.Shutdown(errors.New("test error"))
	})

	// Confirm we can call Shutdown multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			e.Shutdown(errors.New("test error"))
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}

func TestExtensionPurgeBufferedLogs(t *testing.T) {

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			default:
				t.Error("Unknown log type")
			}
			// Mock as if sending logs errored
			return "", "", false, errors.New("server rejected logs")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()

	// Create these buckets ahead of time
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.ResultLogsStore.String())
	agentbbolt.NewStore(multislogger.NewNopLogger(), db, storage.StatusLogsStore.String())

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("BboltDB").Return(db)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	max := 10
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{MaxBufferedLogs: max})
	require.Nil(t, err)

	var expectedStatusLogs, expectedResultLogs []string
	for i := 0; i < 100; i++ {
		gotStatusLogs = nil
		gotResultLogs = nil
		statusLog := fmt.Sprintf("status %d", i)
		expectedStatusLogs = append(expectedStatusLogs, statusLog)
		e.LogString(context.Background(), logger.LogTypeStatus, statusLog)

		resultLog := fmt.Sprintf("result %d", i)
		expectedResultLogs = append(expectedResultLogs, resultLog)
		e.LogString(context.Background(), logger.LogTypeString, resultLog)

		e.writeAndPurgeLogs()

		if i < max {
			assert.Equal(t, expectedStatusLogs, gotStatusLogs)
			assert.Equal(t, expectedResultLogs, gotResultLogs)
		} else {
			assert.Equal(t, expectedStatusLogs[i-max:], gotStatusLogs)
			assert.Equal(t, expectedResultLogs[i-max:], gotResultLogs)
		}
	}
}

func TestExtensionGetQueriesTransportError(t *testing.T) {

	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			return nil, false, errors.New("transport")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	queries, err := e.GetQueries(context.Background())
	assert.True(t, m.RequestQueriesFuncInvoked)
	assert.NotNil(t, err)
	assert.Nil(t, queries)
}

func TestExtensionGetQueriesEnrollmentInvalid(t *testing.T) {

	expectedNodeKey := "good_node_key"
	var gotNodeKey string
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			gotNodeKey = nodeKey
			return nil, true, nil
		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Return("enroll_secret", nil)

	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)
	e.NodeKey = "bad_node_key"

	queries, err := e.GetQueries(context.Background())
	assert.True(t, m.RequestQueriesFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.NotNil(t, err)
	assert.Nil(t, queries)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionGetQueries(t *testing.T) {

	expectedQueries := map[string]string{
		"time":    "select * from time",
		"version": "select version from osquery_info",
	}
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			return &distributed.GetQueriesResult{
				Queries: expectedQueries,
			}, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	queries, err := e.GetQueries(context.Background())
	assert.True(t, m.RequestQueriesFuncInvoked)
	require.Nil(t, err)
	assert.Equal(t, expectedQueries, queries.Queries)
}

func TestExtensionWriteResultsTransportError(t *testing.T) {

	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			return "", "", false, errors.New("transport")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	err = e.WriteResults(context.Background(), []distributed.Result{})
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.NotNil(t, err)
}

func TestExtensionWriteResultsEnrollmentInvalid(t *testing.T) {

	expectedNodeKey := "good_node_key"
	var gotNodeKey string
	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			gotNodeKey = nodeKey
			return "", "", true, nil
		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)
	e.NodeKey = "bad_node_key"

	err = e.WriteResults(context.Background(), []distributed.Result{})
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionWriteResults(t *testing.T) {

	var gotResults []distributed.Result
	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			gotResults = results
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	k := makeKnapsack(t, db)
	e, err := NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.Nil(t, err)

	expectedResults := []distributed.Result{
		{
			QueryName: "foobar",
			Status:    0,
			Rows:      []map[string]string{{"foo": "bar"}},
		},
	}

	err = e.WriteResults(context.Background(), expectedResults)
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedResults, gotResults)
}

func TestLauncherRsaKeys(t *testing.T) {
	m := &mock.KolideService{}

	configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err)
	require.NoError(t, err)

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(configStore)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	_, err = NewExtension(context.TODO(), m, k, ExtensionOpts{})
	require.NoError(t, err)

	key, err := PrivateRSAKeyFromDB(configStore)
	require.NoError(t, err)

	pubkeyPem, fingerprintStored, err := PublicRSAKeyFromDB(configStore)
	require.NoError(t, err)

	fingerprint, err := rsaFingerprint(key)
	require.NoError(t, err)
	require.Equal(t, fingerprint, fingerprintStored)

	pubkey, err := KeyFromPem([]byte(pubkeyPem))
	require.NoError(t, err)

	require.Equal(t, &key.PublicKey, pubkey)
}

func Test_setVerbose(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name           string
		initialConfig  map[string]any
		osqueryVerbose bool
		expectedConfig map[string]any
	}

	testCases := make([]testCase, 0)
	for _, osqVerbose := range []bool{true, false} {
		testCases = append(testCases,
			testCase{
				name:           fmt.Sprintf("empty config, osquery verbose %v", osqVerbose),
				initialConfig:  make(map[string]any),
				osqueryVerbose: osqVerbose,
				expectedConfig: map[string]any{
					"options": map[string]any{
						"verbose": osqVerbose,
					},
				},
			},
			testCase{
				name: fmt.Sprintf("config with verbose already set, osquery verbose %v", osqVerbose),
				initialConfig: map[string]any{
					"options": map[string]any{
						"verbose": !osqVerbose,
					},
				},
				osqueryVerbose: osqVerbose,
				expectedConfig: map[string]any{
					"options": map[string]any{
						"verbose": osqVerbose,
					},
				},
			},
			testCase{
				name: fmt.Sprintf("config with other options, osquery verbose %v", osqVerbose),
				initialConfig: map[string]any{
					"options": map[string]any{
						"audit_allow_config": false,
					},
				},
				osqueryVerbose: osqVerbose,
				expectedConfig: map[string]any{
					"options": map[string]any{
						"audit_allow_config": false,
						"verbose":            osqVerbose,
					},
				},
			},
			testCase{
				name: fmt.Sprintf("config with decorators, osquery verbose %v", osqVerbose),
				initialConfig: map[string]any{
					"decorators": map[string]any{
						"load": []any{
							"SELECT version FROM osquery_info;",
							"SELECT uuid AS host_uuid FROM system_info;",
						},
						"always": []any{
							"SELECT user AS username FROM logged_in_users WHERE user <> '' ORDER BY time LIMIT 1;",
						},
						"interval": map[string]any{
							"3600": []any{"SELECT total_seconds AS uptime FROM uptime;"},
						},
					},
				},
				osqueryVerbose: osqVerbose,
				expectedConfig: map[string]any{
					"options": map[string]any{
						"verbose": osqVerbose,
					},
					"decorators": map[string]any{
						"load": []any{
							"SELECT version FROM osquery_info;",
							"SELECT uuid AS host_uuid FROM system_info;",
						},
						"always": []any{
							"SELECT user AS username FROM logged_in_users WHERE user <> '' ORDER BY time LIMIT 1;",
						},
						"interval": map[string]any{
							"3600": []any{"SELECT total_seconds AS uptime FROM uptime;"},
						},
					},
				},
			},
			testCase{
				name: fmt.Sprintf("config with auto table construction, osquery verbose %v", osqVerbose),
				initialConfig: map[string]any{
					"auto_table_construction": map[string]any{
						"tcc_system_entries": map[string]any{
							"query": "SELECT service, client, auth_value, last_modified FROM access;",
							"path":  "/Library/Application Support/com.apple.TCC/TCC.db",
							"columns": []any{
								"service",
								"client",
								"auth_value",
								"last_modified",
							},
							"platform": "darwin",
						},
					},
				},
				osqueryVerbose: osqVerbose,
				expectedConfig: map[string]any{
					"options": map[string]any{
						"verbose": osqVerbose,
					},
					"auto_table_construction": map[string]any{
						"tcc_system_entries": map[string]any{
							"query": "SELECT service, client, auth_value, last_modified FROM access;",
							"path":  "/Library/Application Support/com.apple.TCC/TCC.db",
							"columns": []any{
								"service",
								"client",
								"auth_value",
								"last_modified",
							},
							"platform": "darwin",
						},
					},
				},
			},
		)
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := &Extension{
				slogger: multislogger.NewNopLogger(),
			}

			cfgBytes, err := json.Marshal(tt.initialConfig)
			require.NoError(t, err)

			modifiedCfgStr := e.setVerbose(string(cfgBytes), tt.osqueryVerbose)

			var modifiedCfg map[string]any
			require.NoError(t, json.Unmarshal([]byte(modifiedCfgStr), &modifiedCfg))

			require.Equal(t, tt.expectedConfig, modifiedCfg)
		})
	}
}

func Test_setVerbose_EmptyConfig(t *testing.T) {
	t.Parallel()

	e := &Extension{
		slogger: multislogger.NewNopLogger(),
	}

	expectedCfg := map[string]any{
		"options": map[string]any{
			"verbose": true,
		},
	}

	modifiedCfgStr := e.setVerbose("", true)

	var modifiedCfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(modifiedCfgStr), &modifiedCfg))

	require.Equal(t, expectedCfg, modifiedCfg)
}

func Test_setVerbose_MalformedConfig(t *testing.T) {
	t.Parallel()

	e := &Extension{
		slogger: multislogger.NewNopLogger(),
	}

	malformedCfg := map[string]any{
		"options": "options should not be a string, yet it is, oops",
	}
	cfgBytes, err := json.Marshal(malformedCfg)
	require.NoError(t, err)
	modifiedCfgStr := e.setVerbose(string(cfgBytes), true)

	var modifiedCfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(modifiedCfgStr), &modifiedCfg))

	require.Equal(t, malformedCfg, modifiedCfg)
}
