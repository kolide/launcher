package localserver

import (
	"bytes"
	"crypto/rsa"
	"io"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/kolide/krypto"
)

type kryptoInterface interface {
	Encode(inResponseTo string, data []byte) (string, error)
	EncodePng(inResponseTo string, data []byte, w io.Writer) error
	DecodeRaw(data []byte) (*krypto.Box, error)
}

// kryptoBoxerMiddleware provides http middleware wrappers over the kryto pkg.
type kryptoBoxerMiddleware struct {
	boxer  kryptoInterface
	logger log.Logger
}

// NewKryptoBoxerMiddleware returns a new kryptoBoxerMiddleware
func NewKryptoBoxerMiddleware(logger log.Logger, myKey *rsa.PrivateKey, serverKey *rsa.PublicKey) (*kryptoBoxerMiddleware, error) {

	kbrw := &kryptoBoxerMiddleware{
		boxer:  krypto.NewBoxer(myKey, serverKey),
		logger: logger,
	}

	return kbrw, nil

}

func (kbm *kryptoBoxerMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the response
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, r)

		requestId := r.URL.Query().Get("id")

		// process the response into a krypto box
		enc, err := kbm.boxer.Encode(requestId, bhr.Bytes())
		if err != nil {
			panic(err)
		}
		w.Write([]byte(enc))
	})
}

func (kbm *kryptoBoxerMiddleware) WrapPng(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the response
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, r)

		requestId := r.URL.Query().Get("id")

		// process the response into a krypto box
		err := kbm.boxer.EncodePng(requestId, bhr.Bytes(), w)
		if err != nil {
			panic(err)
		}
	})
}

type bufferedHttpResponse struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (bhr *bufferedHttpResponse) Header() http.Header {
	return bhr.header
}

func (bhr *bufferedHttpResponse) Write(in []byte) (int, error) {
	return bhr.buf.Write(in)
}

func (bhr *bufferedHttpResponse) WriteHeader(code int) {
	bhr.code = code
}

func (bhr *bufferedHttpResponse) Bytes() []byte {
	return bhr.buf.Bytes()
}
