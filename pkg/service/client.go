package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
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
func New(conn *grpc.ClientConn, logger log.Logger) KolideService {
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
	insecureGRPC bool,
	certPins [][]byte,
	rootPool *x509.CertPool,
	logger log.Logger,
	opts ...grpc.DialOption, // Used for overrides in testing
) (*grpc.ClientConn, error) {
	level.Info(logger).Log(
		"msg", "dialing grpc server",
		"server", serverURL,
		"tls_secure", insecureTLS == false,
		"grpc_secure", insecureGRPC == false,
		"cert_pinning", len(certPins) > 0,
	)
	grpcOpts := []grpc.DialOption{
		grpc.WithTimeout(time.Second),
	}
	if insecureGRPC {
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

func makeTLSConfig(host string, insecureTLS bool, certPins [][]byte, rootPool *x509.CertPool, logger log.Logger) *tls.Config {
	conf := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureTLS,
		RootCAs:            rootPool,
	}

	if len(certPins) > 0 {
		conf.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			for _, chain := range verifiedChains {
				for _, cert := range chain {
					// Compare SHA256 hash of
					// SubjectPublicKeyInfo with each of
					// the pinned hashes.
					hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
					for _, pin := range certPins {
						if bytes.Equal(pin, hash[:]) {
							// Cert matches pin.
							return nil
						}
					}
				}
			}

			// Normally we wouldn't log and return an error, but
			// gRPC does not seem to expose the error in a way that
			// we can get at it later. At least this provides some
			// feedback to the user about what is going wrong.
			level.Info(logger).Log(
				"msg", "no match found with pinned certificates",
				"err", "certificate pin validationf failed",
			)
			return errors.New("no match found with pinned cert")
		}
	}

	return conf
}
