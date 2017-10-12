package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/agent-api"
	"github.com/kolide/launcher/service"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type mockApiServer struct {
	kolide_agent.ApiServer
}

func (m *mockApiServer) RequestEnrollment(context.Context, *kolide_agent.EnrollmentRequest) (*kolide_agent.EnrollmentResponse, error) {
	return &kolide_agent.EnrollmentResponse{}, nil
}

func startServer(t *testing.T, conf *tls.Config) func() {
	grpcServer := grpc.NewServer()
	kolide_agent.RegisterApiServer(grpcServer, &mockApiServer{})
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

const badCert = "testdata/bad-cert.pem"
const badKey = "testdata/bad-key.pem"

const goodCert = "testdata/good-cert.pem"
const goodKey = "testdata/good-key.pem"

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

	conn, err := dialGRPC("localhost:8443", false, false, log.NewNopLogger(),
		grpc.WithTransportCredentials(&tlsCreds{credentials.NewTLS(&tls.Config{RootCAs: pool})}),
	)
	require.Nil(t, err)
	defer conn.Close()

	client := service.New(conn, log.NewNopLogger())

	_, _, err = client.RequestEnrollment(context.Background(), "", "")
	require.NotNil(t, err)

	stop()

	cert, err = tls.LoadX509KeyPair(goodCert, goodKey)
	require.Nil(t, err)
	stop = startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	time.Sleep(1 * time.Second)

	_, _, err = client.RequestEnrollment(context.Background(), "", "")
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

	conn, err := dialGRPC("localhost:8443", false, false, log.NewNopLogger(),
		grpc.WithTransportCredentials(&tlsCreds{credentials.NewTLS(&tls.Config{RootCAs: pool})}),
	)
	require.Nil(t, err)
	defer conn.Close()

	client := service.New(conn, log.NewNopLogger())

	_, _, err = client.RequestEnrollment(context.Background(), "", "")
	require.NotNil(t, err)

	stop()

	cert, err = tls.LoadX509KeyPair(badCert, badKey)
	require.Nil(t, err)
	stop = startServer(t, &tls.Config{Certificates: []tls.Certificate{cert}})
	time.Sleep(1 * time.Second)

	// Should still fail with bad cert
	_, _, err = client.RequestEnrollment(context.Background(), "", "")
	require.NotNil(t, err)

	stop()
}
