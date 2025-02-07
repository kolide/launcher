package localserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()

	c, err := storageci.NewStore(t, slogger, storage.ConfigStore.String())
	require.NoError(t, err)
	require.NoError(t, osquery.SetupLauncherKeys(c))

	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("ConfigStore").Return(c)
	k.On("Slogger").Return(slogger)

	// Override the poll and recalculate interval for the test so we can be sure that the async workers
	// do run, but then stop running on shutdown
	pollInterval = 2 * time.Second
	recalculateInterval = 100 * time.Millisecond

	// Create the localserver
	ls, err := New(context.TODO(), k, nil)
	require.NoError(t, err)

	// Set the querier
	querier := mocks.NewQuerier(t)
	// On a 2-sec interval, letting the server run for 3 seconds, we should see only one query
	querier.On("Query", mock.Anything).Return(nil, nil).Once()
	ls.SetQuerier(querier)

	// Let the server run for a bit
	go ls.Start()
	time.Sleep(3 * time.Second)
	ls.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			ls.Interrupt(nil)
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

	k.AssertExpectations(t)
	querier.AssertExpectations(t)
}

func TestMunemoCheckHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                            string
		headers                         map[string]string
		tokenClaims                     jwt.MapClaims
		expectedStatus                  int
		expectedCallsToReadEnrollSecret int
	}{
		{
			name:                            "matching munemo",
			headers:                         map[string]string{"X-Kolide-Munemo": "test-munemo"},
			tokenClaims:                     jwt.MapClaims{"organization": "test-munemo"},
			expectedStatus:                  http.StatusOK,
			expectedCallsToReadEnrollSecret: 1,
		},
		{
			name:                            "no munemo header",
			tokenClaims:                     jwt.MapClaims{"organization": "test-munemo"},
			expectedStatus:                  http.StatusOK,
			expectedCallsToReadEnrollSecret: 0,
		},
		{
			name:                            "no token claims",
			headers:                         map[string]string{"X-Kolide-Munemo": "test-munemo"},
			expectedStatus:                  http.StatusOK,
			expectedCallsToReadEnrollSecret: 2,
		},
		{
			name:                            "token claim not string",
			headers:                         map[string]string{"X-Kolide-Munemo": "test-munemo"},
			tokenClaims:                     jwt.MapClaims{"organization": 1},
			expectedStatus:                  http.StatusOK,
			expectedCallsToReadEnrollSecret: 2,
		},
		{
			name:                            "empty org claim",
			headers:                         map[string]string{"X-Kolide-Munemo": "test-munemo"},
			tokenClaims:                     jwt.MapClaims{"organization": ""},
			expectedStatus:                  http.StatusOK,
			expectedCallsToReadEnrollSecret: 2,
		},
		{
			name:                            "header and munemo dont match",
			headers:                         map[string]string{"X-Kolide-Munemo": "test-munemo"},
			tokenClaims:                     jwt.MapClaims{"organization": "other-munemo"},
			expectedStatus:                  http.StatusUnauthorized,
			expectedCallsToReadEnrollSecret: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tt.tokenClaims).SignedString([]byte("test"))
			require.NoError(t, err)

			knapsack := typesmocks.NewKnapsack(t)
			if tt.expectedCallsToReadEnrollSecret > 0 {
				knapsack.On("ReadEnrollSecret").Return(token, nil).Times(tt.expectedCallsToReadEnrollSecret)
			}

			server := &localServer{
				knapsack: knapsack,
				slogger:  multislogger.NewNopLogger(),
			}

			// Create a test handler for the middleware to call
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Wrap the test handler in the middleware
			handlerToTest := server.munemoCheckHandler(nextHandler)

			// Create a request with the specified headers
			req := httptest.NewRequest("GET", "/", nil)

			for key, value := range tt.headers {
				req.Header.Add(key, value)
			}

			rr := httptest.NewRecorder()
			handlerToTest.ServeHTTP(rr, req)

			require.Equal(t, tt.expectedStatus, rr.Code)

			// run it again to make sure we actually cached the munemo
			handlerToTest.ServeHTTP(rr, req)
			require.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
