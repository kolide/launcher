package localserver

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func Test_localServer_requestIdHandler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := testBboltDb(t)

			var logBytes bytes.Buffer
			server := testServer(t, &logBytes, db)

			req, err := http.NewRequest("", "", nil)
			require.NoError(t, err)

			handler := http.HandlerFunc(server.requestIdHandlerFunc)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Empty(t, logBytes.String())
			assert.Equal(t, http.StatusOK, rr.Code)

			// convert the response to a struct
			var response requestIdsResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))

			assert.GreaterOrEqual(t, len(response.ConsoleUsers), 1, "should have at least one console user")
		})
	}
}

func testServer(t *testing.T, logBytes *bytes.Buffer, db *bbolt.DB) *localServer {
	server, err := New(log.NewLogfmtLogger(logBytes), db, "")
	require.NoError(t, err)
	return server
}

func testBboltDb(t *testing.T) *bbolt.DB {
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "local_server_test.db"), 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	require.NoError(t, err)

	err = db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("config"))
		require.NoError(t, err)

		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(t, err)

		err = bucket.Put([]byte("privateKey"), keyBytes)
		require.NoError(t, err)

		return nil
	})

	return db
}
