package localserver

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/krypto/pkg/challenge"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/pkg/agent/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKryptoEcMiddleware(t *testing.T) {
	t.Parallel()

	counterpartyKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	challengeId := []byte(ulid.New())
	challengeData := []byte(ulid.New())

	cmdReqBody := []byte(randomStringWithSqlCharacters(t, 100000))

	cmdReq := mustMarshal(t, v2CmdRequestType{
		Path: "whatevs",
		Body: cmdReqBody,
	})

	var tests = []struct {
		name                    string
		localDbKey, hardwareKey crypto.Signer
		challenge               func() ([]byte, *[32]byte)
		loggedErr               string
	}{
		{
			name:       "no command",
			localDbKey: ecdsaKey(t),
			challenge:  func() ([]byte, *[32]byte) { return []byte(""), nil },
			loggedErr:  "no data in box query parameter",
		},
		{
			name:       "no signature",
			localDbKey: ecdsaKey(t),
			challenge:  func() ([]byte, *[32]byte) { return []byte("aGVsbG8gd29ybGQK"), nil },
			loggedErr:  "unable to unmarshal box",
		},
		{
			name:       "malformed cmd",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, _, err := challenge.Generate(counterpartyKey, challengeId, challengeData, []byte("malformed stuff"))
				require.NoError(t, err)
				return challenge, nil
			},
			loggedErr: "unable to unmarshal cmd request",
		},
		{
			name:       "wrong signature",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				malloryKey, err := echelper.GenerateEcdsaKey()
				require.NoError(t, err)
				challenge, _, err := challenge.Generate(malloryKey, challengeId, challengeData, cmdReq)
				return challenge, nil
			},
			loggedErr: "unable to verify signature",
		},
		{
			name:        "works with hardware key",
			localDbKey:  ecdsaKey(t),
			hardwareKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
		},
		{
			name:       "works with nil hardware key",
			localDbKey: ecdsaKey(t),
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
		},
		{
			name:        "works with noop hardware key",
			localDbKey:  ecdsaKey(t),
			hardwareKey: keys.Noop,
			challenge: func() ([]byte, *[32]byte) {
				challenge, priv, err := challenge.Generate(counterpartyKey, challengeId, challengeData, cmdReq)
				require.NoError(t, err)
				return challenge, priv
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer

			// set up middlewares
			kryptoEcMiddleware := newKryptoEcMiddleware(log.NewLogfmtLogger(&logBytes), tt.localDbKey, tt.hardwareKey, counterpartyKey.PublicKey)
			require.NoError(t, err)

			challengeBytes, privateEncryptionKey := tt.challenge()

			// generate the response we want the handler to return
			responseData := []byte(ulid.New())

			// this handler is what will respond to the request made by the kryptoEcMiddleware.Wrap handler
			// in this test we just want it to regurgitate the response data we defined above
			// this should match the responseData in the opened response
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqBodyRaw, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				defer r.Body.Close()

				require.Equal(t, cmdReqBody, reqBodyRaw)
				w.Write(responseData)
			})

			// give our middleware with the test handler to the determiner
			h := NewKryptoDeterminerMiddleware(log.NewLogfmtLogger(&logBytes), nil, kryptoEcMiddleware.Wrap(testHandler))

			encodedChallenge := base64.StdEncoding.EncodeToString(challengeBytes)
			for _, req := range []*http.Request{makeGetRequest(t, encodedChallenge), makePostRequest(t, encodedChallenge)} {
				req := req
				t.Run(req.Method, func(t *testing.T) {
					t.Parallel()

					rr := httptest.NewRecorder()
					h.ServeHTTP(rr, req)

					if tt.loggedErr != "" {
						assert.Equal(t, http.StatusUnauthorized, rr.Code)
						assert.Contains(t, logBytes.String(), tt.loggedErr)
						return
					}

					require.Equal(t, http.StatusOK, rr.Code)
					require.NotEmpty(t, rr.Body.String())

					require.Equal(t, kolideKryptoEccHeader20230130Value, rr.Header().Get(kolideKryptoHeaderKey))

					// try to open the response
					returnedResponseBytes, err := base64.StdEncoding.DecodeString(rr.Body.String())
					require.NoError(t, err)

					responseUnmarshalled, err := challenge.UnmarshalResponse(returnedResponseBytes)
					require.NoError(t, err)
					require.Equal(t, challengeId, responseUnmarshalled.ChallengeId)

					opened, err := responseUnmarshalled.Open(*privateEncryptionKey)
					require.NoError(t, err)
					require.Equal(t, challengeData, opened.ChallengeData)
					require.Equal(t, responseData, opened.ResponseData)
					require.WithinDuration(t, time.Now(), time.Unix(opened.Timestamp, 0), time.Second*5)
				})
			}
		})
	}
}

func ecdsaKey(t *testing.T) *ecdsa.PrivateKey {
	key, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)
	return key
}

// tried to add all the characters that may appear in sql
const randomStringCharsetForSqlEncoding = "aA0_'%!@#&()-[{}]:;',?/*`~$^+=<>\""

func randomStringWithSqlCharacters(t *testing.T, n int) string {
	maxInt := big.NewInt(int64(len(randomStringCharsetForSqlEncoding)))

	sb := strings.Builder{}
	sb.Grow(n)
	for i := 0; i < n; i++ {
		char, err := rand.Int(rand.Reader, maxInt)
		require.NoError(t, err)

		sb.WriteByte(randomStringCharsetForSqlEncoding[int(char.Int64())])
	}
	return sb.String()
}

// just searched for a gnarly sql statement to makes sure encoding can handle it
// https://dev.to/tyzia/example-of-complex-sql-query-to-get-as-much-data-as-possible-from-database-9he
const bigSql = `
SELECT
  e.employee_id AS "Employee #"
  , e.first_name || ' ' || e.last_name AS "Name"
  , e.email AS "Email"
  , e.phone_number AS "Phone"
  , TO_CHAR(e.hire_date, 'MM/DD/YYYY') AS "Hire Date"
  , TO_CHAR(e.salary, 'L99G999D99', 'NLS_NUMERIC_CHARACTERS = ''.,'' NLS_CURRENCY = ''$''') AS "Salary"
  , e.commission_pct AS "Commission %"
  , 'works as ' || j.job_title || ' in ' || d.department_name || ' department (manager: '
    || dm.first_name || ' ' || dm.last_name || ') and immediate supervisor: ' || m.first_name || ' ' || m.last_name AS "Current Job"
  , TO_CHAR(j.min_salary, 'L99G999D99', 'NLS_NUMERIC_CHARACTERS = ''.,'' NLS_CURRENCY = ''$''') || ' - ' ||
      TO_CHAR(j.max_salary, 'L99G999D99', 'NLS_NUMERIC_CHARACTERS = ''.,'' NLS_CURRENCY = ''$''') AS "Current Salary"
  , l.street_address || ', ' || l.postal_code || ', ' || l.city || ', ' || l.state_province || ', '
    || c.country_name || ' (' || r.region_name || ')' AS "Location"
  , jh.job_id AS "History Job ID"
  , 'worked from ' || TO_CHAR(jh.start_date, 'MM/DD/YYYY') || ' to ' || TO_CHAR(jh.end_date, 'MM/DD/YYYY') ||
    ' as ' || jj.job_title || ' in ' || dd.department_name || ' department' AS "History Job Title"

FROM employees e
-- to get title of current job_id
  JOIN jobs j
    ON e.job_id = j.job_id
-- to get name of current manager_id
  LEFT JOIN employees m
    ON e.manager_id = m.employee_id
-- to get name of current department_id
  LEFT JOIN departments d
    ON d.department_id = e.department_id
-- to get name of manager of current department
-- (not equal to current manager and can be equal to the employee itself)
  LEFT JOIN employees dm
    ON d.manager_id = dm.employee_id
-- to get name of location
  LEFT JOIN locations l
    ON d.location_id = l.location_id
  LEFT JOIN countries c
    ON l.country_id = c.country_id
  LEFT JOIN regions r
    ON c.region_id = r.region_id
-- to get job history of employee
  LEFT JOIN job_history jh
    ON e.employee_id = jh.employee_id
-- to get title of job history job_id
  LEFT JOIN jobs jj
    ON jj.job_id = jh.job_id
-- to get namee of department from job history
  LEFT JOIN departments dd
    ON dd.department_id = jh.department_id

ORDER BY e.employee_id;
`
