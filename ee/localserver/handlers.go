package localserver

import (
	"bytes"
	"fmt"
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

func (krw *kryptoBoxResponseWriter) Unwrap(next http.Handler) http.Handler {
	// TODO maybe check max body size before we do this? Or implement something streaming.
	// On the other hand, the requests are coming from localhost...
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("HI")

		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("Failed to io.Copy: %v", err)
			w.WriteHeader(401)
			return
		} else if len(body) == 0 {
			fmt.Println("No data in request", err)
			w.WriteHeader(401)
			return
		}

		decoded, err := krw.boxer.DecodeRaw(body)
		if err != nil {
			//level.Debug(ls.logger).Log("msg", "Unable to decode request", "err", err)
			fmt.Println("Unable to decode request", err)
			w.WriteHeader(401)
			return
		}

		r.Body = closingBuffer{bytes.NewBuffer(decoded.Data())}
		next.ServeHTTP(w, r)
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

type closingBuffer struct {
	*bytes.Buffer
}

func (cb closingBuffer) Close() error { return nil }
