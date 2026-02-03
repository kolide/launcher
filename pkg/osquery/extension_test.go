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
	"net/http"
	"sort"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/kit/testutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/osquerypublisher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	settingsstoremock "github.com/kolide/launcher/pkg/osquery/mocks"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/service"
	"github.com/kolide/launcher/pkg/service/mock"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// makeKnapsack returns a types.Knapsack ready for use in most tests. Use this when your test
// expects the extension to already be enrolled. If you need an unenrolled extension, use makeKnapsackUnenrolled.
// If you need to manipulate the enrollment state (e.g. by changing node keys), then set up the mock knapsack
// manually in your test instead.
func makeKnapsack(t *testing.T) types.Knapsack {
	m := mocks.NewKnapsack(t)
	m.On("OsquerydPath").Maybe().Return("")
	m.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	m.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	m.On("Slogger").Return(multislogger.NewNopLogger())
	m.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	m.On("RootDirectory").Maybe().Return("whatever")
	m.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	m.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	m.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	m.On("OsqueryHistory").Return(osqHistory).Maybe()
	m.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	m.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	m.On("NodeKey", testifymock.Anything).Return(ulid.New(), nil).Maybe()
	// for now, don't enable dual log publication (cutover to new agent-ingester service) for these
	// tests. that logic is tested separately and we can add more logic to test here if needed once
	// we've settled on a cutover plan and desired behaviors
	m.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	m.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	m.On("TokenStore").Return(tokenStore).Maybe()
	return m
}

func makeTestOsqLogPublisher(k types.Knapsack) types.OsqueryPublisher {
	slogger := multislogger.NewNopLogger()
	return osquerypublisher.NewLogPublisherClient(slogger, k, http.DefaultClient)
}

// makeKnapsackUnenrolled returns a types.Knapsack ready for use in any test that requires
// an unenrolled extension. If you need to manipulate the enrollment state (e.g. by changing
// node keys), then set up the mock knapsack manually in your test instead.
func makeKnapsackUnenrolled(t *testing.T) types.Knapsack {
	m := mocks.NewKnapsack(t)
	m.On("OsquerydPath").Maybe().Return("")
	m.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	m.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	m.On("Slogger").Return(multislogger.NewNopLogger())
	m.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	m.On("RootDirectory").Maybe().Return("whatever")
	m.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	m.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	m.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	m.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	m.On("OsqueryHistory").Return(osqHistory).Maybe()
	m.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	m.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	m.On("NodeKey", testifymock.Anything).Return("", nil).Maybe()
	// for now, don't enable dual log publication (cutover to new agent-ingester service) for these
	// tests. that logic is tested separately and we can add more logic to test here if needed once
	// we've settled on a cutover plan and desired behaviors
	m.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	m.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	m.On("TokenStore").Return(tokenStore).Maybe()
	return m
}

// makeKnapsackWithInvalidEnrollment returns aa types.Knapsack ready for use in any test that requires
// an extension with an invalid node key, to test reenrollment.
func makeKnapsackWithInvalidEnrollment(t *testing.T, expectedNodeKey string) types.Knapsack {
	// Set up our knapsack
	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()

	// At first, return a bad node key -- this will be called once by whatever function we're calling.
	k.On("NodeKey", testifymock.Anything).Return("bad_node_key", nil).Once()
	// We expect that we'll attempt to delete any existing enrollment before attempting reenroll.
	k.On("DeleteEnrollment", testifymock.Anything).Return(nil)
	// On re-enroll, we'll check to confirm that we don't have a node key (perhaps from a different enroll thread).
	// Return no node key, to confirm we proceed with reenrollment.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Once()
	// Post-enrollment, we'll save the enrollment.
	k.On("SaveEnrollment", testifymock.Anything, "", expectedNodeKey, testifymock.Anything).Return(nil).Once()
	// Next, post-enrollment, we'll want to start returning the correct node key.
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)
	// for now, don't enable dual log publication (cutover to new agent-ingester service) for these
	// tests. that logic is tested separately and we can add more logic to test here if needed once
	// we've settled on a cutover plan and desired behaviors
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	k.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	return k
}

func TestNewExtensionEmptyEnrollSecret(t *testing.T) {
	m := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(m)

	// We should be able to make an extension despite an empty enroll secret
	e, err := NewExtension(t.Context(), &mock.KolideService{}, lpc, settingsstoremock.NewSettingsStoreWriter(t), m, ulid.New(), ExtensionOpts{})
	assert.Nil(t, err)
	assert.NotNil(t, e)
}

func TestGetHostIdentifier(t *testing.T) {
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), &mock.KolideService{}, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	ident, err := e.getHostIdentifier()
	require.Nil(t, err)
	assert.True(t, len(ident) > 10)
	oldIdent := ident

	ident, err = e.getHostIdentifier()
	require.Nil(t, err)
	assert.Equal(t, oldIdent, ident)

	k = makeKnapsack(t)
	e, err = NewExtension(t.Context(), &mock.KolideService{}, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})

	require.Nil(t, err)

	ident, err = e.getHostIdentifier()
	require.Nil(t, err)
	// Should get different UUID with fresh DB
	assert.NotEqual(t, oldIdent, ident)
}

func TestGetHostIdentifierCorruptedData(t *testing.T) {
	// Put bad data in the DB and ensure we can still generate a fresh UUID
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), &mock.KolideService{}, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return nil, errors.New("transport")
		},
	}

	k := makeKnapsackUnenrolled(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(t.Context())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnrollSecretInvalid(t *testing.T) {
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeInvalid: true,
			}, nil
		},
	}
	k := makeKnapsackUnenrolled(t)
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(t.Context())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

// createTestEnrollSecret creates a JWT that can be parsed by the extension
// to extract its munemo.
func createTestEnrollSecret(t *testing.T, munemo string) string {
	testSigningKey := []byte("test-key")

	type CustomKolideJwtClaims struct {
		Munemo string `json:"organization"`
		jwt.RegisteredClaims
	}

	claims := CustomKolideJwtClaims{
		munemo,
		jwt.RegisteredClaims{
			// A usual scenario is to set the expiration time relative to the current time
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedTokenStr, err := token.SignedString(testSigningKey)
	require.NoError(t, err)

	return signedTokenStr
}

func TestExtensionEnrollValidNodeEmptyResponse(t *testing.T) {
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return nil, nil
		},
	}
	k := makeKnapsackUnenrolled(t)
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(t.Context())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.False(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnrollInvalidRegion(t *testing.T) {
	expectedDeviceServerURL := "device.example.test"
	expectedControlServerURL := "control.example.test"
	expectedOsqueryPublisherURL := "pub.example.test"

	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				RegionInvalid: true,
				RegionURLs: &types.KolideURLs{
					DeviceServerURL:     expectedDeviceServerURL,
					ControlServerURL:    expectedControlServerURL,
					OsqueryPublisherURL: expectedOsqueryPublisherURL,
				},
			}, nil
		},
	}

	expectedMunemo := "test_fake_munemo_2"
	expectedEnrollSecret := createTestEnrollSecret(t, expectedMunemo)
	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err)
	k.On("ConfigStore").Return(configStore)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return(expectedEnrollSecret, nil)
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	// We should attempt to fetch the node key once during enrollment, and we shouldn't have a node key yet
	k.On("NodeKey", types.DefaultEnrollmentID).Return("", nil).Once()

	// We should update the regional URLs
	k.On("SetKolideServerURL", expectedDeviceServerURL).Return(nil)
	k.On("SetControlServerURL", expectedControlServerURL).Return(nil)
	k.On("SetOsqueryPublisherURL", expectedOsqueryPublisherURL).Return(nil)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(t.Context())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.False(t, invalid)
	assert.NotNil(t, err)

	k.AssertExpectations(t)
}

func TestExtensionEnrollInvalidRegion_DoesNotSetMissingUrls(t *testing.T) {
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				RegionInvalid: true,
				RegionURLs: &types.KolideURLs{
					DeviceServerURL:     "",
					ControlServerURL:    "",
					OsqueryPublisherURL: "",
				},
			}, nil
		},
	}

	expectedMunemo := "test_fake_munemo_2"
	expectedEnrollSecret := createTestEnrollSecret(t, expectedMunemo)
	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err)
	k.On("ConfigStore").Return(configStore)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return(expectedEnrollSecret, nil)
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	// We should attempt to fetch the node key once during enrollment, and we shouldn't have a node key yet
	k.On("NodeKey", types.DefaultEnrollmentID).Return("", nil).Once()

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	key, invalid, err := e.Enroll(t.Context())
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.False(t, invalid)
	assert.NotNil(t, err)

	k.AssertExpectations(t)
}

func TestExtensionEnroll(t *testing.T) {
	expectedMunemo := "test_fake_munemo"
	expectedEnrollSecret := createTestEnrollSecret(t, expectedMunemo)

	var gotEnrollSecret string
	expectedNodeKey := "node_key"
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			gotEnrollSecret = enrollSecret
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}
	s := settingsstoremock.NewSettingsStoreWriter(t)

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err)
	k.On("ConfigStore").Return(configStore)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return(expectedEnrollSecret, nil)
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), m, lpc, s, k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	// We should attempt to fetch the node key once during enrollment, and we shouldn't have a node key yet
	k.On("NodeKey", types.DefaultEnrollmentID).Return("", nil).Once()
	// We expect enrollment to complete, and that we store the updated enrollment
	k.On("SaveEnrollment", types.DefaultEnrollmentID, testifymock.Anything, expectedNodeKey, expectedEnrollSecret).Return(nil).Once()

	// Attempt enrollment
	key, invalid, err := e.Enroll(t.Context())
	require.Nil(t, err)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	// The next enroll request will have access to the new node key
	k.On("NodeKey", types.DefaultEnrollmentID).Return(expectedNodeKey, nil).Once()

	// The next time we call Enroll, the extension should confirm that the enrollment is stored
	k.On("EnsureEnrollmentStored", types.DefaultEnrollmentID).Return(nil)

	// Should not re-enroll with stored secret
	m.RequestEnrollmentFuncInvoked = false
	key, invalid, err = e.Enroll(t.Context())
	require.Nil(t, err)
	assert.False(t, m.RequestEnrollmentFuncInvoked) // Note False here.
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	e, err = NewExtension(t.Context(), m, lpc, s, k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)
	// Still should not re-enroll (because node key stored in DB)
	k.On("NodeKey", types.DefaultEnrollmentID).Return(expectedNodeKey, nil).Once()
	key, invalid, err = e.Enroll(t.Context())
	require.Nil(t, err)
	assert.False(t, m.RequestEnrollmentFuncInvoked) // Note False here.
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	// Re-enroll for new node key
	expectedNodeKey = "new_node_key"
	k.On("SaveEnrollment", types.DefaultEnrollmentID, "", expectedNodeKey, expectedEnrollSecret).Return(nil).Once()
	k.On("DeleteEnrollment", types.DefaultEnrollmentID).Return(nil)
	k.On("NodeKey", types.DefaultEnrollmentID).Return("", nil).Once()
	e.RequireReenroll(t.Context())
	key, invalid, err = e.Enroll(t.Context())
	require.Nil(t, err)
	// Now enroll func should be called again
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)

	k.AssertExpectations(t)
}

func TestExtensionGenerateConfigsTransportError(t *testing.T) {
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return "", false, errors.New("transport")
		},
	}
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(t.Context())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Nil(t, configs)
	// An error with the cache empty should be returned
	assert.NotNil(t, err)
}

func TestExtensionGenerateConfigsCaching(t *testing.T) {
	configVal := `{"foo":"bar","options":{"distributed_interval":5,"verbose":true}}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil)
	e, err := NewExtension(t.Context(), m, lpc, s, k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(t.Context())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Equal(t, map[string]string{"config": configVal}, configs)
	assert.Nil(t, err)

	// Now have requesting the config fail, and expect to get the same
	// config anyway (through the cache).
	m.RequestConfigFuncInvoked = false
	m.RequestConfigFunc = func(ctx context.Context, nodeKey string) (string, bool, error) {
		return "", false, errors.New("foobar")
	}
	configs, err = e.GenerateConfigs(t.Context())
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
			return "", true, nil // node_invalid
		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}
	// Set up our knapsack
	k := makeKnapsackWithInvalidEnrollment(t, expectedNodeKey)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(t.Context())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Nil(t, configs)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestGenerateConfigs_CannotEnrollYet(t *testing.T) {
	expectedNodeKey := "new_node_key"
	configVal := `{"foo":"bar","options":{"distributed_interval":5,"verbose":true}}`
	s := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			if nodeKey == expectedNodeKey {
				return configVal, false, nil
			}
			// Returns node_invalid
			return "", true, nil
		},
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Return("", errors.New("test")).Once() // checked once in Enroll
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	settingsStore := settingsstoremock.NewSettingsStoreWriter(t)
	settingsStore.On("WriteSettings").Return(nil)
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), s, lpc, settingsStore, k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	// First, simulate no enrollment secret being available yet, with launcher not yet enrolled.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Times(3) // called once when making the RequestConfig call, once at the top of Enroll, and once on a final `enrolled` check during RequestConfig error handling
	configs, err := e.GenerateConfigs(t.Context())
	assert.NotNil(t, configs)
	assert.Equal(t, map[string]string{"config": "{}"}, configs)
	assert.Nil(t, err)

	// Since we can't retrieve the enroll secret, we shouldn't attempt to enroll yet
	require.False(t, s.RequestEnrollmentFuncInvoked)

	// Now, make an enrollment secret available, and prepare for enrollment
	k.On("ReadEnrollSecret").Return("test_enrollment_secret", nil)
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil)
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()

	// Post-enrollment, we'll save the enrollment.
	k.On("SaveEnrollment", testifymock.Anything, "", expectedNodeKey, testifymock.Anything).Return(nil).Once()

	// We need NodeKey to return empty twice more to trigger reenrollment:
	// once on the initial call to RequestConfigs, and once at the top of Enroll.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Times(2)

	// Post-enrollment, we should now have the new node key available.
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	// Make a config request again -- we should, this time, successfully attempt a reenroll
	updatedConfigs, err := e.GenerateConfigs(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]string{"config": configVal}, updatedConfigs)

	// We should have enrolled
	require.True(t, s.RequestEnrollmentFuncInvoked)

	// Should have tried to request config
	assert.True(t, s.RequestConfigFuncInvoked)

	// On node invalid response, should attempt to retrieve enroll secret
	k.AssertExpectations(t)
}

func TestGenerateConfigs_WorksAfterSecretlessEnrollment(t *testing.T) {
	nodeKeyFromSecretlessEnrollment := "new_node_key_from_secretless_enrollment"
	configVal := `{"foo":"bar","options":{"distributed_interval":5,"verbose":true}}`
	s := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			if nodeKey != nodeKeyFromSecretlessEnrollment {
				// Returns node_invalid
				return "", true, nil
			}
			return configVal, false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err, configStore)
	k.On("ConfigStore").Return(configStore, nil)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("", errors.New("test"))
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	settingsStore := settingsstoremock.NewSettingsStoreWriter(t)
	settingsStore.On("WriteSettings").Return(nil)
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), s, lpc, settingsStore, k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	// First request to generate configs -- we shouldn't be able to get anything yet,
	// since we haven't enrolled.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Times(3) // called once when making the RequestConfig call, once at the top of Enroll, and once on a final `enrolled` check during RequestConfig error handling
	configs, err := e.GenerateConfigs(t.Context())
	assert.NotNil(t, configs)
	assert.Equal(t, map[string]string{"config": "{}"}, configs)
	assert.Nil(t, err)

	// We should not have tried to request enrollment, since this is a secretless installation;
	// we should not have tried to call RequestConfig, since we don't have a node key yet.
	assert.False(t, s.RequestEnrollmentFuncInvoked)
	assert.False(t, s.RequestConfigFuncInvoked)

	// Now, set the node key, to simulate secretless enrollment completing in a different thread.
	k.On("NodeKey", testifymock.Anything).Return(nodeKeyFromSecretlessEnrollment, nil)

	// Try to generate configs again. This time, we should use the correct node key.
	newConfigs, err := e.GenerateConfigs(t.Context())
	assert.True(t, s.RequestConfigFuncInvoked)
	assert.Equal(t, map[string]string{"config": configVal}, newConfigs)
	assert.Nil(t, err)

	// On node invalid response, should attempt to retrieve enroll secret
	k.AssertExpectations(t)

	// Since we don't have an enroll secret, we shouldn't attempt to enroll
	assert.False(t, s.RequestEnrollmentFuncInvoked)
}

func TestExtensionGenerateConfigs(t *testing.T) {
	configVal := `{"foo":"bar","options":{"distributed_interval":5,"verbose":true}}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	k := makeKnapsack(t)
	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, s, k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	configs, err := e.GenerateConfigs(t.Context())
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
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	err = e.writeLogsWithReenroll(t.Context(), logger.LogTypeSnapshot, []string{"foobar"}, true)
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}
	// Set up our knapsack
	k := makeKnapsackWithInvalidEnrollment(t, expectedNodeKey)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	err = e.writeLogsWithReenroll(t.Context(), logger.LogTypeString, []string{"foobar"}, true)
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

	// Set up our knapsack
	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()

	// Set our node key
	expectedNodeKey := "node_key"
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	err = e.writeLogsWithReenroll(t.Context(), logger.LogTypeStatus, []string{"foobar"}, true)
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

	// Create the status logs bucket ahead of time
	statusLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.StatusLogsStore.String())
	require.NoError(t, err)

	k := mocks.NewKnapsack(t)
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger()).Maybe()
	k.On("StatusLogsStore").Return(statusLogsStore)
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	// No buffered logs should result in success and no remote action being
	// taken.
	err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	assert.Nil(t, err)
	assert.False(t, m.PublishLogsFuncInvoked)
}

func TestExtensionWriteBufferedLogs(t *testing.T) {
	expectedNodeKey := ulid.New()

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			if nodeKey != expectedNodeKey {
				return "", "", false, nil // invalid
			}

			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			case logger.LogTypeSnapshot, logger.LogTypeHealth, logger.LogTypeInit:
				t.Errorf("unexpected log type %v", logType)
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	k.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	// Create these buckets ahead of time
	statusLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.StatusLogsStore.String())
	require.NoError(t, err)
	k.On("StatusLogsStore").Return(statusLogsStore)
	resultLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ResultLogsStore.String())
	require.NoError(t, err)
	k.On("ResultLogsStore").Return(resultLogsStore)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	e.LogString(t.Context(), logger.LogTypeStatus, "status foo")
	e.LogString(t.Context(), logger.LogTypeStatus, "status bar")

	e.LogString(t.Context(), logger.LogTypeString, "result foo")
	e.LogString(t.Context(), logger.LogTypeString, "result bar")

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

	e.LogString(t.Context(), logger.LogTypeStatus, "status foo")

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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}

	// Set up our knapsack
	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	k.On("OsqueryPublisherURL").Return("").Maybe()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()

	// Create the status logs bucket ahead of time
	statusLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.StatusLogsStore.String())
	require.NoError(t, err)
	k.On("StatusLogsStore").Return(statusLogsStore)
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	// At first, return a bad node key -- this will be called once by GenerateConfigs.
	k.On("NodeKey", testifymock.Anything).Return("bad_node_key", nil).Once()
	// We expect that we'll attempt to delete any existing enrollment before attempting reenroll.
	k.On("DeleteEnrollment", testifymock.Anything).Return(nil)
	// On re-enroll, we'll check to confirm that we don't have a node key (perhaps from a different enroll thread).
	// Return no node key, to confirm we proceed with reenrollment.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Once()
	// Post-enrollment, we'll save the enrollment.
	k.On("SaveEnrollment", testifymock.Anything, "", expectedNodeKey, testifymock.Anything).Return(nil).Once()
	// Next, post-enrollment, we'll want to start returning the correct node key.
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	e.LogString(t.Context(), logger.LogTypeStatus, "status foo")
	e.LogString(t.Context(), logger.LogTypeStatus, "status bar")

	// long timeout is due to github actions runners IO slowness
	testutil.FatalAfterFunc(t, 7*time.Second, func() {
		err = e.writeBufferedLogsForType(logger.LogTypeStatus)
	})
	assert.Nil(t, err)
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionWriteBufferedLogsLimit(t *testing.T) {
	expectedNodeKey := ulid.New()

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			if nodeKey != expectedNodeKey {
				return "", "", false, nil // invalid
			}

			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			case logger.LogTypeSnapshot, logger.LogTypeHealth, logger.LogTypeInit:
				t.Errorf("unexpected log type %v", logType)
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	k.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	// Create these buckets ahead of time
	statusLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.StatusLogsStore.String())
	require.NoError(t, err)
	k.On("StatusLogsStore").Return(statusLogsStore)
	resultLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ResultLogsStore.String())
	require.NoError(t, err)
	k.On("ResultLogsStore").Return(resultLogsStore)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBytesPerBatch: 100,
	})
	require.Nil(t, err)

	expectedStatusLogs := []string{}
	expectedResultLogs := []string{}
	for i := range 20 {
		status := fmt.Sprintf("status_%3d", i)
		expectedStatusLogs = append(expectedStatusLogs, status)
		e.LogString(t.Context(), logger.LogTypeStatus, status)

		result := fmt.Sprintf("result_%3d", i)
		expectedResultLogs = append(expectedResultLogs, result)
		e.LogString(t.Context(), logger.LogTypeString, result)
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
	expectedNodeKey := ulid.New()

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			if nodeKey != expectedNodeKey {
				return "", "", false, nil // invalid
			}

			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			case logger.LogTypeSnapshot, logger.LogTypeHealth, logger.LogTypeInit:
				t.Errorf("unexpected log type %v", logType)
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	k.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	// Create these buckets ahead of time
	resultLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ResultLogsStore.String())
	require.NoError(t, err)
	k.On("ResultLogsStore").Return(resultLogsStore)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBytesPerBatch: 15,
	})
	require.Nil(t, err)

	startLogCount, err := e.knapsack.ResultLogsStore().Count()
	require.NoError(t, err)
	require.Equal(t, 0, startLogCount, "start with no buffered logs")

	expectedResultLogs := []string{"res1", "res2", "res3", "res4"}
	e.LogString(t.Context(), logger.LogTypeString, "this_result_is_tooooooo_big! oh noes")
	e.LogString(t.Context(), logger.LogTypeString, "res1")
	e.LogString(t.Context(), logger.LogTypeString, "res2")
	e.LogString(t.Context(), logger.LogTypeString, "this_result_is_tooooooo_big! wow")
	e.LogString(t.Context(), logger.LogTypeString, "this_result_is_tooooooo_big! scheiße")
	e.LogString(t.Context(), logger.LogTypeString, "res3")
	e.LogString(t.Context(), logger.LogTypeString, "res4")
	e.LogString(t.Context(), logger.LogTypeString, "this_result_is_tooooooo_big! darn")

	queuedLogCount, err := e.knapsack.ResultLogsStore().Count()
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

	finalLogCount, err := e.knapsack.ResultLogsStore().Count()
	require.NoError(t, err)
	require.Equal(t, 0, finalLogCount, "no more queued logs")
}

func TestExtensionWriteLogsLoop(t *testing.T) {
	expectedNodeKey := ulid.New()
	var gotStatusLogs, gotResultLogs []string
	var logLock sync.Mutex
	var done = make(chan struct{})
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			defer func() { done <- struct{}{} }()

			if nodeKey != expectedNodeKey {
				return "", "", false, nil // invalid
			}

			logLock.Lock()
			defer logLock.Unlock()

			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			case logger.LogTypeSnapshot, logger.LogTypeHealth, logger.LogTypeInit:
				t.Errorf("unexpected log type %v", logType)
			default:
				t.Error("Unknown log type")
			}
			return "", "", false, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	k.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	k.On("OsqueryPublisherURL").Return("").Maybe()
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)
	k.On("EnsureEnrollmentStored", testifymock.Anything).Return(nil)

	// Create these buckets ahead of time
	statusLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.StatusLogsStore.String())
	require.NoError(t, err)
	k.On("StatusLogsStore").Return(statusLogsStore)
	resultLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ResultLogsStore.String())
	require.NoError(t, err)
	k.On("ResultLogsStore").Return(resultLogsStore)

	expectedLoggingInterval := 5 * time.Second
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBytesPerBatch: 200,
		LoggingInterval:  expectedLoggingInterval,
	})
	require.Nil(t, err)

	expectedStatusLogs := []string{}
	expectedResultLogs := []string{}
	for i := range 20 {
		status := fmt.Sprintf("status_%013d", i)
		expectedStatusLogs = append(expectedStatusLogs, status)
		e.LogString(t.Context(), logger.LogTypeStatus, status)

		result := fmt.Sprintf("result_%013d", i)
		expectedResultLogs = append(expectedResultLogs, result)
		e.LogString(t.Context(), logger.LogTypeString, result)
	}

	// Should write first 10 logs
	go e.Execute()
	testutil.FatalAfterFunc(t, 1*time.Second, func() {
		// PublishLogsFunc runs twice for each run of the loop
		<-done
		<-done
	})
	// Examine the logs, then reset them for the next test
	logLock.Lock()
	assert.Equal(t, expectedStatusLogs[:10], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[:10], gotResultLogs)
	gotStatusLogs = nil
	gotResultLogs = nil
	logLock.Unlock()

	// Should write last 10 logs
	time.Sleep(expectedLoggingInterval + 1*time.Second)
	testutil.FatalAfterFunc(t, 1*time.Second, func() {
		// PublishLogsFunc runs twice of each run of the loop
		<-done
		<-done
	})
	// Examine the logs, then reset them for the next test
	logLock.Lock()
	assert.Equal(t, expectedStatusLogs[10:], gotStatusLogs)
	assert.Equal(t, expectedResultLogs[10:], gotResultLogs)
	gotStatusLogs = nil
	gotResultLogs = nil
	logLock.Unlock()

	// No more logs to write
	time.Sleep(expectedLoggingInterval + 1*time.Second)
	// Block to ensure publish function could be called if the logic is
	// incorrect
	time.Sleep(1 * time.Millisecond)
	// Confirm logs are nil because nothing got published
	logLock.Lock()
	assert.Nil(t, gotStatusLogs)
	assert.Nil(t, gotResultLogs)
	logLock.Unlock()

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
	for receivedInterrupts < expectedInterrupts {
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
	expectedNodeKey := ulid.New()

	var gotStatusLogs, gotResultLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			if nodeKey != expectedNodeKey {
				return "", "", false, nil // invalid
			}

			switch logType {
			case logger.LogTypeStatus:
				gotStatusLogs = logs
			case logger.LogTypeString:
				gotResultLogs = logs
			case logger.LogTypeSnapshot, logger.LogTypeHealth, logger.LogTypeInit:
				t.Errorf("unexpected log type %v", logType)
			default:
				t.Error("Unknown log type")
			}
			// Mock as if sending logs errored
			return "", "", false, errors.New("server rejected logs")
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	// Create these buckets ahead of time
	statusLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.StatusLogsStore.String())
	require.NoError(t, err)
	k.On("StatusLogsStore").Return(statusLogsStore)
	resultLogsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ResultLogsStore.String())
	require.NoError(t, err)
	k.On("ResultLogsStore").Return(resultLogsStore)

	maximum := 10
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{
		MaxBufferedLogs: maximum,
	})
	require.Nil(t, err)

	var expectedStatusLogs, expectedResultLogs []string
	for i := range 100 {
		gotStatusLogs = nil
		gotResultLogs = nil
		statusLog := fmt.Sprintf("status %d", i)
		expectedStatusLogs = append(expectedStatusLogs, statusLog)
		e.LogString(t.Context(), logger.LogTypeStatus, statusLog)

		resultLog := fmt.Sprintf("result %d", i)
		expectedResultLogs = append(expectedResultLogs, resultLog)
		e.LogString(t.Context(), logger.LogTypeString, resultLog)

		e.writeAndPurgeLogs()

		if i < maximum {
			assert.Equal(t, expectedStatusLogs, gotStatusLogs)
			assert.Equal(t, expectedResultLogs, gotResultLogs)
		} else {
			assert.Equal(t, expectedStatusLogs[i-maximum:], gotStatusLogs)
			assert.Equal(t, expectedResultLogs[i-maximum:], gotResultLogs)
		}
	}
}

func TestExtensionGetQueriesTransportError(t *testing.T) {
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			return nil, false, errors.New("transport")
		},
	}
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	queries, err := e.GetQueries(t.Context())
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}

	// Set up our knapsack
	k := makeKnapsackWithInvalidEnrollment(t, expectedNodeKey)
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	queries, err := e.GetQueries(t.Context())
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
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	queries, err := e.GetQueries(t.Context())
	assert.True(t, m.RequestQueriesFuncInvoked)
	require.Nil(t, err)
	assert.Equal(t, expectedQueries, queries.Queries)
}

func TestGetQueries_Forwarding(t *testing.T) {
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
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	// Shorten our accelerated startup period so we aren't waiting for a full two minutes for the test
	acceleratedStartupPeriod := 10 * time.Second
	e.forwardAllDistributedUntil.Store(time.Now().Unix() + int64(acceleratedStartupPeriod.Seconds()))

	// Shorten the forwarding interval for the same reason
	e.distributedForwardingInterval.Store(5)

	// Request queries -- this first time, since we're requesting queries right after startup,
	// the request should go through.
	queries, err := e.GetQueries(t.Context())
	assert.True(t, m.RequestQueriesFuncInvoked)
	require.Nil(t, err)
	assert.Equal(t, expectedQueries, queries.Queries)

	// Now, wait for the startup period to pass
	time.Sleep(acceleratedStartupPeriod)

	// Request queries -- this time, the request should go through because we haven't made a request
	// within the last `e.distributedForwardingInterval` seconds.
	queries, err = e.GetQueries(t.Context())
	require.Nil(t, err)
	require.Equal(t, expectedQueries, queries.Queries)

	// Immediately request queries again. This time, the request should NOT go through, because
	// we made a request within the last `e.distributedForwardingInterval` seconds.
	// We should get back an empty response.
	queries, err = e.GetQueries(t.Context())
	require.Nil(t, err)
	require.Nil(t, queries.Queries)
}

func TestGetQueries_Forwarding_RespondsToAccelerationRequest(t *testing.T) {
	expectedQueries := map[string]string{
		"time":    "select * from time",
		"version": "select version from osquery_info",
	}
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			return &distributed.GetQueriesResult{
				Queries:           expectedQueries,
				AccelerateSeconds: 30, // set AccelerateSeconds to make sure the extension responds accordingly
			}, false, nil
		},
	}
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	// Shorten our accelerated startup period so we aren't waiting for a full two minutes to start the test
	e.forwardAllDistributedUntil.Store(0)

	// Request queries -- this first time, since we're requesting queries right after startup,
	// the request should go through.
	queries, err := e.GetQueries(t.Context())
	assert.True(t, m.RequestQueriesFuncInvoked)
	require.Nil(t, err)
	assert.Equal(t, expectedQueries, queries.Queries)

	// We should have responded to the `AccelerateSeconds` set by the response.
	// Confirm that we set `e.forwardAllDistributedUntil` to accelerate requests.
	require.Greater(t, e.forwardAllDistributedUntil.Load(), int64(0))

	// Request queries twice more -- this will be before e.distributedForwardingInterval seconds have passed,
	// but we should be in accelerated mode and should therefore see the request go through to the cloud.
	queries, err = e.GetQueries(t.Context())
	require.Nil(t, err)
	require.Equal(t, expectedQueries, queries.Queries)

	queries, err = e.GetQueries(t.Context())
	require.Nil(t, err)
	require.Equal(t, expectedQueries, queries.Queries)
}

func TestGetQueries_WorksWithSecretlessEnrollment(t *testing.T) {
	nodeKeyFromSecretlessEnrollment := "another_new_node_key_from_secretless_enrollment"
	expectedQueries := map[string]string{
		"time":    "select * from time",
		"version": "select version from osquery_info",
	}
	s := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			if nodeKey == nodeKeyFromSecretlessEnrollment {
				return &distributed.GetQueriesResult{
					Queries: expectedQueries,
				}, false, nil
			}

			return nil, true, nil
		},
	}

	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err, configStore)
	k.On("ConfigStore").Return(configStore, nil)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("", errors.New("test"))
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	settingsStore := settingsstoremock.NewSettingsStoreWriter(t)
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), s, lpc, settingsStore, k, types.DefaultEnrollmentID, ExtensionOpts{})
	require.Nil(t, err)

	// First request to generate configs -- we shouldn't be able to get anything yet,
	// since we haven't enrolled.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Times(2) // called once when making the GetQueries call, once at the top of Enroll
	_, err = e.GetQueries(t.Context())
	require.Error(t, err, "should not have been able to get queries while unenrolled")

	// Should not have tried to enroll, since we don't have a secret;
	// should not have tried to call RequestQueries, since we don't have a node key.
	assert.False(t, s.RequestEnrollmentFuncInvoked)
	assert.False(t, s.RequestQueriesFuncInvoked)

	// Now, fire off a bunch of sequential GetQueries requests, so that we'll (probably) have one
	// processing while we simulate secretless enrollment completing
	resultChan := make(chan struct{}, 100)
	go func() {
		for range 100 {
			_, _ = e.GetQueries(t.Context())
			resultChan <- struct{}{}
		}
	}()

	// Now, set the node key, to simulate secretless enrollment completing in a different thread.
	k.On("NodeKey", testifymock.Anything).Return(nodeKeyFromSecretlessEnrollment, nil)

	// Wait for our previous queries to complete
	for range 100 {
		select {
		case <-resultChan:
			// Nothing to do here
		case <-time.After(1 * time.Minute):
			t.FailNow()
		}
	}

	// Now, try to get queries -- we should be enrolled, so we should be able to get
	// queries with the correct node key now.
	newQueries, err := e.GetQueries(t.Context())
	require.NoError(t, err)
	require.Equal(t, expectedQueries, newQueries.Queries)

	// Since we don't have an enroll secret, we shouldn't ever attempt to enroll
	assert.False(t, s.RequestEnrollmentFuncInvoked)

	k.AssertExpectations(t)
}

func TestExtensionWriteResultsTransportError(t *testing.T) {
	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			return "", "", false, errors.New("transport")
		},
	}
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	err = e.WriteResults(t.Context(), []distributed.Result{})
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
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (*service.EnrollmentResponse, error) {
			return &service.EnrollmentResponse{
				NodeKey:            expectedNodeKey,
				NodeInvalid:        false,
				AgentIngesterToken: "",
			}, nil
		},
	}

	// Set up our knapsack
	k := mocks.NewKnapsack(t)
	k.On("OsquerydPath").Maybe().Return("")
	k.On("LatestOsquerydPath", testifymock.Anything).Maybe().Return("")
	k.On("ConfigStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String()))
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("ReadEnrollSecret").Maybe().Return("enroll_secret", nil)
	k.On("RootDirectory").Maybe().Return("whatever")
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", testifymock.Anything, testifymock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", testifymock.Anything).Maybe().Return()
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	k.On("TokenStore").Return(tokenStore).Maybe()
	lpc := makeTestOsqLogPublisher(k)

	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	// At first, return a bad node key -- this will be called once by WriteResults.
	k.On("NodeKey", testifymock.Anything).Return("bad_node_key", nil).Once()
	// We expect that we'll attempt to delete any existing enrollment before attempting reenroll.
	k.On("DeleteEnrollment", testifymock.Anything).Return(nil)
	// On re-enroll, we'll check to confirm that we don't have a node key (perhaps from a different enroll thread).
	// Return no node key, to confirm we proceed with reenrollment.
	k.On("NodeKey", testifymock.Anything).Return("", nil).Once()
	// Post-enrollment, we'll save the enrollment.
	k.On("SaveEnrollment", testifymock.Anything, "", expectedNodeKey, testifymock.Anything).Return(nil).Once()
	// Next, post-enrollment, we'll want to start returning the correct node key.
	k.On("NodeKey", testifymock.Anything).Return(expectedNodeKey, nil)

	err = e.WriteResults(t.Context(), []distributed.Result{})
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
	k := makeKnapsack(t)
	lpc := makeTestOsqLogPublisher(k)
	e, err := NewExtension(t.Context(), m, lpc, settingsstoremock.NewSettingsStoreWriter(t), k, ulid.New(), ExtensionOpts{})
	require.Nil(t, err)

	expectedResults := []distributed.Result{
		{
			QueryName: "foobar",
			Status:    0,
			Rows:      []map[string]string{{"foo": "bar"}},
		},
	}

	err = e.WriteResults(t.Context(), expectedResults)
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedResults, gotResults)
}

func Test_setOsqueryOptions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name           string
		initialConfig  map[string]any
		overrideOpts   map[string]any
		expectedConfig map[string]any
	}

	testCases := []testCase{
		{
			name:          "empty config, startup override opts",
			initialConfig: make(map[string]any),
			overrideOpts:  startupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              true,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name:          "empty config, post-startup override opts",
			initialConfig: make(map[string]any),
			overrideOpts:  postStartupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              false,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name: "config with verbose already set, startup override opts",
			initialConfig: map[string]any{
				"options": map[string]any{
					"verbose": false,
				},
			},
			overrideOpts: startupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              true,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name: "config with verbose already set, post-startup override opts",
			initialConfig: map[string]any{
				"options": map[string]any{
					"verbose": true,
				},
			},
			overrideOpts: postStartupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              false,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name: "config with distributed_interval already set, startup override opts",
			initialConfig: map[string]any{
				"options": map[string]any{
					"distributed_interval": 30,
				},
			},
			overrideOpts: startupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              true,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name: "config with distributed_interval already set, post-startup override opts",
			initialConfig: map[string]any{
				"options": map[string]any{
					"distributed_interval": 25,
				},
			},
			overrideOpts: postStartupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              false,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name: "config with other options, startup override opts",
			initialConfig: map[string]any{
				"options": map[string]any{
					"audit_allow_config": false,
				},
			},
			overrideOpts: startupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"audit_allow_config":   false,
					"verbose":              true,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
				},
			},
		},
		{
			name: "config with decorators, post-startup override opts",
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
			overrideOpts: postStartupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              false,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
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
		{
			name: "config with auto table construction, startup override opts",
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
			overrideOpts: startupOsqueryConfigOptions,
			expectedConfig: map[string]any{
				"options": map[string]any{
					"verbose":              true,
					"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
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
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := &Extension{
				slogger: multislogger.NewNopLogger(),
			}

			cfgBytes, err := json.Marshal(tt.initialConfig)
			require.NoError(t, err)

			modifiedCfgStr := e.setOsqueryOptions(string(cfgBytes), tt.overrideOpts)

			var modifiedCfg map[string]any
			require.NoError(t, json.Unmarshal([]byte(modifiedCfgStr), &modifiedCfg))

			require.Equal(t, tt.expectedConfig, modifiedCfg)
		})
	}
}

func Test_setOsqueryOptions_EmptyConfig(t *testing.T) {
	t.Parallel()

	e := &Extension{
		slogger: multislogger.NewNopLogger(),
	}

	expectedCfg := map[string]any{
		"options": map[string]any{
			"verbose":              true,
			"distributed_interval": float64(osqueryDistributedInterval), // has to be a float64 due to json unmarshal nonsense
		},
	}

	modifiedCfgStr := e.setOsqueryOptions("", startupOsqueryConfigOptions)

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
	modifiedCfgStr := e.setOsqueryOptions(string(cfgBytes), postStartupOsqueryConfigOptions)

	var modifiedCfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(modifiedCfgStr), &modifiedCfg))

	require.Equal(t, malformedCfg, modifiedCfg)
}
