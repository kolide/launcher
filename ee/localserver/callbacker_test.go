package localserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallbacker(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/hello", strings.NewReader("i am a body"))
	require.NoError(t, err)
	req.Header.Add("X-hello", "hi")
	req.Header.Add("X-ctx", "example.com/postme")

	handler := CallbackWrap(http.HandlerFunc(mockHandler))
	handler.ServeHTTP(rr, req)

	assert.Equal(t, 200, rr.Code)
	assert.Equal(t, "hi", rr.Header().Get("X-hello"))
	assert.Equal(t, "i am a body", rr.Body.String())

}

func TestMockHandler(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/hello", strings.NewReader("i am a body"))
	require.NoError(t, err)
	req.Header.Add("X-hello", "hi")
	req.Header.Add("X-status", "503")

	mockHandler(rr, req)

	assert.Equal(t, 503, rr.Code)
	assert.Equal(t, "hi", rr.Header().Get("X-hello"))
	assert.Equal(t, "i am a body", rr.Body.String())

}

func mockHandler(w http.ResponseWriter, r *http.Request) {
	for h, v := range r.Header {
		h = strings.ToLower(h)
		switch {
		case !strings.HasPrefix(h, "x-"):
			continue
		case strings.HasPrefix(h, "x-ctx") && len(v) > 0 && v[0] != "":
			SetPostbackViaContext(r.Context(), v[0])
		case h == "x-status" && len(v) > 0 && v[0] != "": // If it's the status response, set appropriately.
			i, err := strconv.Atoi(v[0])
			if err != nil {
				// dunno
				continue
			}
			// Set status. With the poorly named "WriteHeader"
			w.WriteHeader(i)
		default:
			// Copy the headers over
			for _, val := range v {
				w.Header().Add(h, val)
			}
		}
	}

	io.Copy(w, r.Body)
}
