package service

import (
	"context"
	"crypto/x509"
	"log/slog"
	"net"
	"strings"
	"time"

	grpctransport "github.com/go-kit/kit/transport/grpc"
	"github.com/kolide/kit/contexts/uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kolide/launcher/ee/agent/types"
	pb "github.com/kolide/launcher/pkg/pb/launcher"
)

// New creates a new Kolide Client (implementation of the KolideService
// interface) using the provided gRPC client connection.
func NewGRPCClient(k types.Knapsack, conn *grpc.ClientConn) KolideService {
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

	client = LoggingMiddleware(k)(client)
	// Wrap with UUID middleware after logger so that UUID is available in
	// the logger context.
	client = uuidMiddleware(client)

	return client
}

// DialGRPC creates a grpc client connection.
func DialGRPC(
	k types.Knapsack,
	rootPool *x509.CertPool,
	opts ...grpc.DialOption, // Used for overrides in testing
) (*grpc.ClientConn, error) {

	k.Slogger().Log(context.TODO(), slog.LevelDebug,
		"dialing grpc server",
		"server", k.KolideServerURL(),
		"tls_secure", !k.InsecureTLS(),
		"transport_secure", !k.InsecureTransportTLS(),
		"cert_pinning", len(k.CertPins()) > 0,
	)

	grpcCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	grpcOpts := []grpc.DialOption{}
	if k.InsecureTransportTLS() {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := &tlsCreds{credentials.NewTLS(makeTLSConfig(k, rootPool))}
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(creds))
	}

	grpcOpts = append(grpcOpts, opts...)

	conn, err := grpc.DialContext(grpcCtx, k.KolideServerURL(), grpcOpts...)
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
