package localserver

import (
	"bytes"
	"io"
	"net/http"

	"github.com/kolide/krypto"
)

type encoderInt interface {
	Encode(inResponseTo string, data []byte) (string, error)
	EncodePng(inResponseTo string, data []byte, w io.Writer) error
	DecodeRaw(data []byte) (*krypto.Box, error)
}

type kryptoBoxResponseWriter struct {
	boxer encoderInt
}

func (krw *kryptoBoxResponseWriter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the response
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, r)

		// process the response into a krypto box
		enc, err := krw.boxer.Encode("", bhr.Bytes())
		if err != nil {
			panic(err)
		}
		w.Write([]byte(enc))
	})
}

func (krw *kryptoBoxResponseWriter) WrapPng(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the response
		bhr := &bufferedHttpResponse{}
		next.ServeHTTP(bhr, r)

		// process the response into a krypto box
		err := krw.boxer.EncodePng("", bhr.Bytes(), w)
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
