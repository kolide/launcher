package service

import (
	"context"
	"crypto/x509"
	"net"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	grpctransport "github.com/go-kit/kit/transport/grpc"
	"github.com/kolide/kit/contexts/uuid"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

// New creates a new Kolide Client (implementation of the KolideService
// interface) using the provided gRPC client connection.
func NewGRPCClient(conn *grpc.ClientConn, logger log.Logger) KolideService {
	requestEnrollmentEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestEnrollment",
		encodeGRPCEnrollmentRequest,
		decodeGRPCEnrollmentResponse,
		pb.EnrollmentResponse{},
		uuid.Attach(),
	).Endpoint()

	requestConfigEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestConfig",
		encodeGRPCConfigRequest,
		decodeGRPCConfigResponse,
		pb.ConfigResponse{},
		uuid.Attach(),
	).Endpoint()

	publishLogsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishLogs",
		encodeGRPCLogCollection,
		decodeGRPCPublishLogsResponse,
		pb.AgentApiResponse{},
		uuid.Attach(),
	).Endpoint()

	requestQueriesEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"RequestQueries",
		encodeGRPCQueriesRequest,
		decodeGRPCQueryCollection,
		pb.QueryCollection{},
		uuid.Attach(),
	).Endpoint()

	publishResultsEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"PublishResults",
		encodeGRPCResultCollection,
		decodeGRPCPublishResultsResponse,
		pb.AgentApiResponse{},
		uuid.Attach(),
	).Endpoint()

	checkHealthEndpoint := grpctransport.NewClient(
		conn,
		"kolide.agent.Api",
		"CheckHealth",
		encodeGRPCHealcheckRequest,
		decodeGRPCHealthCheckResponse,
		pb.HealthCheckResponse{},
		uuid.Attach(),
	).Endpoint()

	var client KolideService = Endpoints{
		RequestEnrollmentEndpoint: requestEnrollmentEndpoint,
		RequestConfigEndpoint:     requestConfigEndpoint,
		PublishLogsEndpoint:       publishLogsEndpoint,
		RequestQueriesEndpoint:    requestQueriesEndpoint,
		PublishResultsEndpoint:    publishResultsEndpoint,
		CheckHealthEndpoint:       checkHealthEndpoint,
	}

	client = LoggingMiddleware(logger)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	return client
}

// dialGRPC creates a grpc client connection.
func DialGRPC(
	serverURL string,
	insecureTLS bool,
	insecureTransport bool,
	certPins [][]byte,
	rootPool *x509.CertPool,
	logger log.Logger,
	opts ...grpc.DialOption, // Used for overrides in testing
) (*grpc.ClientConn, error) {
	level.Info(logger).Log(
		"msg", "dialing grpc server",
		"server", serverURL,
		"tls_secure", insecureTLS == false,
		"transport_secure", insecureTransport == false,
		"cert_pinning", len(certPins) > 0,
	)
	grpcOpts := []grpc.DialOption{
		grpc.WithTimeout(time.Second),
	}
	if insecureTransport {
		grpcOpts = append(grpcOpts, grpc.WithInsecure())
	} else {
		host, _, err := net.SplitHostPort(serverURL)
		if err != nil {
			return nil, errors.Wrapf(err, "split grpc server host and port: %s", serverURL)
		}

		creds := &tlsCreds{credentials.NewTLS(makeTLSConfig(host, insecureTLS, certPins, rootPool, logger))}
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))
	}

	grpcOpts = append(grpcOpts, opts...)

	conn, err := grpc.Dial(serverURL, grpcOpts...)
	return conn, err
}

// tempErr is a wrapper for errors that are "temporary" and should be retried
// for gRPC.
type tempErr struct {
	error
}

func (t tempErr) Temporary() bool {
	return true
}

// tlsCreds overwrites the ClientHandshake method for specific error handling.
type tlsCreds struct {
	credentials.TransportCredentials
}

// ClientHandshake wraps the normal gRPC ClientHandshake, but treats a
// certificate with the wrong name as a temporary rather than permanent error.
// This is important for reconnecting to the gRPC server after, for example,
// the certificate being MitMed by a captive portal (without this, gRPC calls
// will error and never attempt to reconnect).
// See https://github.com/grpc/grpc-go/issues/1571.
func (t *tlsCreds) ClientHandshake(ctx context.Context, s string, c net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn, info, err := t.TransportCredentials.ClientHandshake(ctx, s, c)
	if err != nil && strings.Contains(err.Error(), "x509: certificate is valid for ") {
		err = &tempErr{err}
	}

	return conn, info, err
}
