package service

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type mockApiServer struct {
	KolideService
}

func (m *mockApiServer) RequestEnrollment(ctx context.Context, enrollSecret, hostIdentifier string, details EnrollmentDetails) (string, bool, error) {
	return "", false, nil
}

func startServer(t *testing.T, conf *tls.Config) func() {
	svc := &mockApiServer{}
	logger := log.NewNopLogger()
	e := MakeServerEndpoints(svc)
	apiServer := NewGRPCServer(e, logger)

	grpcServer := grpc.NewServer()
	RegisterGRPCServer(grpcServer, apiServer)
	var listener net.Listener
	var err error
	listener, err = net.Listen("tcp", "localhost:8443")
	require.Nil(t, err)
	listener = tls.NewListener(listener, conf)
	go func() {
		err := grpcServer.Serve(listener)
		if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
			require.Nil(t, err)
		}
	}()
	return grpcServer.Stop
}

//nolint:deadcode
const (
	badCert = "testdata/bad-cert.pem"
	badKey  = "testdata/bad-key.pem"

	goodCert = "testdata/good.crt"
	goodKey  = "testdata/good.key"

	leafCert = "testdata/certchain/leaf.crt"
	leafKey  = "testdata/certchain/leaf.key"

	intermediateCert = "testdata/certchain/intermediate.crt"
	intermediateKey  = "testdata/certchain/intermediate.key"

	rootCert = "testdata/certchain/root.crt"
	rootKey  = "testdata/certchain/root.key"

	chainPem = "testdata/certchain/chain.pem"
)

func calcCertFingerprint(t *testing.T, certpath string) string {
	// openssl rsa -in certchain-old/leaf.key -outform der -pubout | openssl dgst -sha256
	certcontents, err := ioutil.ReadFile(certpath)
	require.NoError(t, err, "reading", certpath)

	block, _ := pem.Decode(certcontents)
	require.NotNil(t, block, "pem decoding", certpath)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "x509.ParseCertificate", certpath)

	digest := sha256.Sum256(cert.RawSubjectPublicKeyInfo)

	return fmt.Sprintf("%x", digest)
}

func TestSwappingCert(t *testing.T) { // nolint:paralleltest
	cert, err := tls.LoadX509KeyPair(badCert, badKey)
	require.Nil(t, err)
	stop := startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	time.Sleep(1 * time.Second)

	pem1, err := ioutil.ReadFile(badCert)
	require.Nil(t, err)
	pem2, err := ioutil.ReadFile(goodCert)
	require.Nil(t, err)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem1)
	pool.AppendCertsFromPEM(pem2)

	conn, err := DialGRPC("localhost:8443", false, false, nil, nil, log.NewNopLogger(),
		grpc.WithTransportCredentials(&tlsCreds{credentials.NewTLS(&tls.Config{RootCAs: pool})}),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := NewGRPCClient(conn, log.NewNopLogger())

	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.Error(t, err)

	stop()

	cert, err = tls.LoadX509KeyPair(goodCert, goodKey)
	require.Nil(t, err)
	stop = startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})

	// Wait for amount of time
	timer := time.NewTimer(time.Second * 2)
	<-timer.C

	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.NoError(t, err)

	stop()
}

func TestCertRemainsBad(t *testing.T) { // nolint:paralleltest
	cert, err := tls.LoadX509KeyPair(badCert, badKey)
	require.Nil(t, err)
	stop := startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})

	// Wait for amount of time
	timer := time.NewTimer(time.Second * 2)
	<-timer.C

	pem1, err := ioutil.ReadFile(badCert)
	require.NoError(t, err)
	pem2, err := ioutil.ReadFile(goodCert)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pem1)
	pool.AppendCertsFromPEM(pem2)

	conn, err := DialGRPC("localhost:8443", false, false, nil, nil, log.NewNopLogger(),
		grpc.WithTransportCredentials(&tlsCreds{credentials.NewTLS(&tls.Config{RootCAs: pool})}),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := NewGRPCClient(conn, log.NewNopLogger())

	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.Error(t, err)

	stop()

	cert, err = tls.LoadX509KeyPair(badCert, badKey)
	require.Nil(t, err)
	stop = startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	time.Sleep(1 * time.Second)

	// Should still fail with bad cert
	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.Error(t, err)

	stop()
}

func TestCertPinning(t *testing.T) { // nolint:paralleltest
	cert, err := tls.LoadX509KeyPair(chainPem, leafKey)
	require.Nil(t, err)
	stop := startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	defer stop()
	time.Sleep(1 * time.Second)

	pem1, err := ioutil.ReadFile(rootCert)
	require.Nil(t, err)
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(pem1)
	require.True(t, ok)

	testCases := []struct {
		pins    []string
		success bool
	}{
		// Success cases
		// pin leaf
		{[]string{calcCertFingerprint(t, leafCert)}, true},
		// pin leaf + extra garbage
		{[]string{"deadb33f", calcCertFingerprint(t, leafCert)}, true},
		// pin intermediate
		{[]string{calcCertFingerprint(t, intermediateCert)}, true},
		// pin root
		{[]string{calcCertFingerprint(t, rootCert)}, true},
		// pin all three
		{[]string{
			calcCertFingerprint(t, rootCert),
			calcCertFingerprint(t, intermediateCert),
			calcCertFingerprint(t, leafCert),
		}, true},

		// Failure cases
		{[]string{"deadb33f"}, false},
		{[]string{"deadb33f", "34567fff"}, false},
		{[]string{"5dc4d2318f1ffabb80d94ad67a6f05ab9f77591ffc131498ed03eef3b5075281"}, false},
	}

	for _, tt := range testCases { // nolint:paralleltest
		t.Run("", func(t *testing.T) {
			certPins, err := parseCertPins(tt.pins)
			require.NoError(t, err)

			tlsconf := makeTLSConfig("localhost", false, certPins, nil, log.NewNopLogger())
			tlsconf.RootCAs = pool

			conn, err := DialGRPC("localhost:8443", false, false, nil, nil, log.NewNopLogger(),
				grpc.WithTransportCredentials(&tlsCreds{credentials.NewTLS(tlsconf)}),
			)
			require.NoError(t, err)
			defer conn.Close()

			client := NewGRPCClient(conn, log.NewNopLogger())

			_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
			if tt.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestRootCAs(t *testing.T) { // nolint:paralleltest
	cert, err := tls.LoadX509KeyPair(chainPem, leafKey)
	require.NoError(t, err)
	stop := startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	defer stop()
	time.Sleep(1 * time.Second)

	rootPEM, err := ioutil.ReadFile(rootCert)
	require.NoError(t, err)
	otherPEM, err := ioutil.ReadFile(goodCert)
	require.NoError(t, err)

	emptyPool := x509.NewCertPool()

	rootPool := x509.NewCertPool()
	ok := rootPool.AppendCertsFromPEM(rootPEM)
	require.True(t, ok)

	otherPool := x509.NewCertPool()
	ok = otherPool.AppendCertsFromPEM(otherPEM)
	require.True(t, ok)

	bothPool := x509.NewCertPool()
	ok = bothPool.AppendCertsFromPEM(otherPEM)
	require.True(t, ok)
	ok = bothPool.AppendCertsFromPEM(rootPEM)
	require.True(t, ok)

	testCases := []struct {
		pool    *x509.CertPool
		success bool
	}{
		// Success cases
		{rootPool, true},
		{bothPool, true},

		// Failure cases
		{emptyPool, false},
		{otherPool, false},
	}

	for _, tt := range testCases { // nolint:paralleltest
		t.Run("", func(t *testing.T) {
			conn, err := DialGRPC("localhost:8443", false, false, nil, tt.pool, log.NewNopLogger())
			require.NoError(t, err)
			defer conn.Close()

			client := NewGRPCClient(conn, log.NewNopLogger())

			_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
			if tt.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func parseCertPins(pins []string) ([][]byte, error) {
	var certPins [][]byte
	if len(pins) > 0 {
		for _, hexPin := range pins {
			pin, err := hex.DecodeString(hexPin)
			if err != nil {
				return nil, errors.Wrap(err, "decoding cert pin")
			}
			certPins = append(certPins, pin)
		}
	}
	return certPins, nil
}
