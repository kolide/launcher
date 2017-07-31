package osquery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/facebookgo/clock"
	"github.com/kolide/launcher/service/mock"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTempDB(t *testing.T) (db *bolt.DB, cleanup func()) {
	file, err := ioutil.TempFile("", "kolide_launcher_test")
	if err != nil {
		t.Fatalf("creating temp file: %s", err.Error())
	}

	db, err = bolt.Open(file.Name(), 0600, nil)
	if err != nil {
		t.Fatalf("opening bolt DB: %s", err.Error())
	}

	return db, func() {
		db.Close()
		os.Remove(file.Name())
	}
}

func TestNewExtensionEmptyEnrollSecret(t *testing.T) {
	e, err := NewExtension(&mock.KolideService{}, nil, ExtensionOpts{})
	assert.NotNil(t, err)
	assert.Nil(t, e)
}

func TestNewExtensionDatabaseError(t *testing.T) {
	file, err := ioutil.TempFile("", "kolide_launcher_test")
	if err != nil {
		t.Fatalf("creating temp file: %s", err.Error())
	}

	db, _ := makeTempDB(t)
	path := db.Path()
	db.Close()

	// Open read-only DB
	db, err = bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("opening bolt DB: %s", err.Error())
	}
	defer func() {
		db.Close()
		os.Remove(file.Name())
	}()

	e, err := NewExtension(&mock.KolideService{}, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	assert.NotNil(t, err)
	assert.Nil(t, e)
}

func TestGetHostIdentifier(t *testing.T) {
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(&mock.KolideService{}, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err = NewExtension(&mock.KolideService{}, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err := NewExtension(&mock.KolideService{}, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)

	// Put garbage UUID in DB
	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(configBucket))
		return b.Put([]byte(uuidKey), []byte("garbage_uuid"))
	})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return "", false, errors.New("transport")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(context.Background())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnrollSecretInvalid(t *testing.T) {
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return "", true, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			gotEnrollSecret = enrollSecret
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	expectedEnrollSecret := "foo_secret"
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: expectedEnrollSecret})
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

	e, err = NewExtension(m, db, ExtensionOpts{EnrollSecret: expectedEnrollSecret})
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Nil(t, configs)
	// An error with the cache empty should be returned
	assert.NotNil(t, err)
}

func TestExtensionGenerateConfigsCaching(t *testing.T) {
	configVal := `{"foo": "bar"}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)
	e.NodeKey = "bad_node_key"

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Nil(t, configs)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionGenerateConfigs(t *testing.T) {
	configVal := `{"foo": "bar"}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)
	e.NodeKey = expectedNodeKey

	err = e.writeLogsWithReenroll(context.Background(), logger.LogTypeStatus, []string{"foobar"}, true)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
	assert.Equal(t, logger.LogTypeStatus, gotLogType)
	assert.Equal(t, []string{"foobar"}, gotLogs)
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)

	// No buffered logs should result in success and no remote action being
	// taken.
	err = e.writeBufferedLogs()
	assert.False(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
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
				t.Error("Unkown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
	require.Nil(t, err)

	e.LogString(context.Background(), logger.LogTypeStatus, "status foo")
	e.LogString(context.Background(), logger.LogTypeStatus, "status bar")

	e.LogString(context.Background(), logger.LogTypeString, "result foo")
	e.LogString(context.Background(), logger.LogTypeString, "result bar")

	err = e.writeBufferedLogs()
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, []string{"status foo", "status bar"}, gotStatusLogs)
	assert.Equal(t, []string{"result foo", "result bar"}, gotResultLogs)

	// No more logs should be written after logs flushed
	m.PublishLogsFuncInvoked = false
	gotStatusLogs = nil
	gotResultLogs = nil
	err = e.writeBufferedLogs()
	assert.False(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)

	e.LogString(context.Background(), logger.LogTypeStatus, "status foo")

	err = e.writeBufferedLogs()
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, []string{"status foo"}, gotStatusLogs)
	assert.Nil(t, gotResultLogs)
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
				t.Error("Unkown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{
		EnrollSecret:    "enroll_secret",
		MaxLogsPerBatch: 10,
	})
	require.Nil(t, err)

	expectedStatusLogs := []string{}
	expectedResultLogs := []string{}
	for i := 0; i < 20; i++ {
		status := fmt.Sprintf("status_%d", i)
		expectedStatusLogs = append(expectedStatusLogs, status)
		e.LogString(context.Background(), logger.LogTypeStatus, status)

		result := fmt.Sprintf("result_%d", i)
		expectedResultLogs = append(expectedResultLogs, result)
		e.LogString(context.Background(), logger.LogTypeString, result)
	}

	// Should write first 10 logs
	err = e.writeBufferedLogs()
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedStatusLogs[:10], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[:10], gotResultLogs)

	// Should write last 10 logs
	m.PublishLogsFuncInvoked = false
	gotStatusLogs = nil
	gotResultLogs = nil
	err = e.writeBufferedLogs()
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedStatusLogs[10:], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[10:], gotResultLogs)

	// No more logs to write
	m.PublishLogsFuncInvoked = false
	gotStatusLogs = nil
	gotResultLogs = nil
	err = e.writeBufferedLogs()
	assert.False(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)
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
				t.Error("Unkown log type")
			}
			return "", "", false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	mockClock := clock.NewMock()
	expectedLoggingInterval := 10 * time.Second
	e, err := NewExtension(m, db, ExtensionOpts{
		EnrollSecret:    "enroll_secret",
		MaxLogsPerBatch: 10,
		Clock:           mockClock,
		LoggingInterval: expectedLoggingInterval,
	})
	require.Nil(t, err)

	expectedStatusLogs := []string{}
	expectedResultLogs := []string{}
	for i := 0; i < 20; i++ {
		status := fmt.Sprintf("status_%d", i)
		expectedStatusLogs = append(expectedStatusLogs, status)
		e.LogString(context.Background(), logger.LogTypeStatus, status)

		result := fmt.Sprintf("result_%d", i)
		expectedResultLogs = append(expectedResultLogs, result)
		e.LogString(context.Background(), logger.LogTypeString, result)
	}

	// Should write first 10 logs
	go e.writeLogsLoop()
	// PublishLogsFunc runs twice of each run of the loop
	<-done
	<-done
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
	mockClock.Add(expectedLoggingInterval + 1*time.Second)
	// PublishLogsFunc runs twice of each run of the loop
	<-done
	<-done
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
	mockClock.Add(expectedLoggingInterval + 1*time.Second)
	// Block to ensure publish function could be called if the logic is
	// incorrect
	time.Sleep(1 * time.Millisecond)
	assert.False(t, funcInvokedStatus)
	assert.False(t, funcInvokedResult)
	assert.Nil(t, err)
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)

	e.Shutdown()
}

func TestExtensionGetQueriesTransportError(t *testing.T) {
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			return nil, false, errors.New("transport")
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return expectedNodeKey, false, nil
		},
	}
	db, cleanup := makeTempDB(t)
	defer cleanup()
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
	e, err := NewExtension(m, db, ExtensionOpts{EnrollSecret: "enroll_secret"})
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
