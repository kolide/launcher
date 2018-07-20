package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
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

const (
	badCert = "testdata/bad-cert.pem"
	badKey  = "testdata/bad-key.pem"

	goodCert = "testdata/good-cert.pem"
	goodKey  = "testdata/good-key.pem"

	leafCert = "testdata/certchain/leaf.crt"
	leafKey  = "testdata/certchain/leaf.key"

	rootCert = "testdata/certchain/root.crt"
	rootKey  = "testdata/certchain/root.key"

	chainPem = "testdata/certchain/chain.pem"
)

func TestSwappingCert(t *testing.T) {
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
	require.Nil(t, err)
	defer conn.Close()

	client := NewClient(conn, log.NewNopLogger())

	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.NotNil(t, err)

	stop()

	cert, err = tls.LoadX509KeyPair(goodCert, goodKey)
	require.Nil(t, err)
	stop = startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	time.Sleep(1 * time.Second)

	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.Nil(t, err)

	stop()
}

func TestCertRemainsBad(t *testing.T) {
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
	require.Nil(t, err)
	defer conn.Close()

	client := NewClient(conn, log.NewNopLogger())

	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.NotNil(t, err)

	stop()

	cert, err = tls.LoadX509KeyPair(badCert, badKey)
	require.Nil(t, err)
	stop = startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	time.Sleep(1 * time.Second)

	// Should still fail with bad cert
	_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
	require.NotNil(t, err)

	stop()
}

func TestCertPinning(t *testing.T) {
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
		pins    string
		success bool
	}{
		// Success cases
		// pin leaf
		{"eb46067da68f80b5d9f0b027985182aa875bcda6c0d8713dbdb8d1523993bd92", true},
		// pin leaf + extra garbage
		{"deadb33f,eb46067da68f80b5d9f0b027985182aa875bcda6c0d8713dbdb8d1523993bd92", true},
		// pin intermediate
		{"73db41a73c5ede78709fc926a2b93e7ad044a40333ce4ce5ae0fb7424620646e", true},
		// pin root
		{"b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b", true},
		// pin all three
		{"b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b,73db41a73c5ede78709fc926a2b93e7ad044a40333ce4ce5ae0fb7424620646e,b48364002b8ac4dd3794d41c204a0282f8cd4f7dc80b26274659512c9619ac1b", true},

		// Failure cases
		{"deadb33f", false},
		{"deadb33f,34567fff", false},
		{"5dc4d2318f1ffabb80d94ad67a6f05ab9f77591ffc131498ed03eef3b5075281", false},
	}

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			certPins, err := parseCertPins(tt.pins)
			require.NoError(t, err)

			tlsconf := makeTLSConfig("localhost", false, certPins, nil, log.NewNopLogger())
			tlsconf.RootCAs = pool

			conn, err := DialGRPC("localhost:8443", false, false, nil, nil, log.NewNopLogger(),
				grpc.WithTransportCredentials(&tlsCreds{credentials.NewTLS(tlsconf)}),
			)
			require.Nil(t, err)
			defer conn.Close()

			client := NewClient(conn, log.NewNopLogger())

			_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
			if tt.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestRootCAs(t *testing.T) {
	cert, err := tls.LoadX509KeyPair(chainPem, leafKey)
	require.Nil(t, err)
	stop := startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	defer stop()
	time.Sleep(1 * time.Second)

	rootPEM, err := ioutil.ReadFile(rootCert)
	require.Nil(t, err)
	otherPEM, err := ioutil.ReadFile(goodCert)
	require.Nil(t, err)

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

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			conn, err := DialGRPC("localhost:8443", false, false, nil, tt.pool, log.NewNopLogger())
			require.Nil(t, err)
			defer conn.Close()

			client := NewClient(conn, log.NewNopLogger())

			_, _, err = client.RequestEnrollment(context.Background(), "", "", EnrollmentDetails{})
			if tt.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func parseCertPins(pins string) ([][]byte, error) {
	var certPins [][]byte
	if pins != "" {
		for _, hexPin := range strings.Split(pins, ",") {
			pin, err := hex.DecodeString(hexPin)
			if err != nil {
				return nil, errors.Wrap(err, "decoding cert pin")
			}
			certPins = append(certPins, pin)
		}
	}
	return certPins, nil
}
