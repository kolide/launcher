package osquery

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/kolide/launcher/service"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/pkg/errors"
)

type Extension struct {
	ServerAddress string
	EnrollSecret  string
	NodeKey       string
	serviceClient service.KolideService
	clientConn    *grpc.ClientConn
}

func NewExtension(serverAddress, enrollSecret, hostIdentifier string) (*Extension, error) {
	// TODO fix insecure
	conn, err := grpc.Dial(serverAddress, grpc.WithInsecure(), grpc.WithTimeout(time.Second))
	if err != nil {
		return nil, errors.Wrap(err, "dialing grpc server")
	}

	client := service.New(conn)

	key, invalid, err := client.RequestEnrollment(context.Background(), enrollSecret, hostIdentifier)
	if err != nil {
		conn.Close()
		return nil, errors.Wrap(err, "transport error in enrollment")
	}
	if invalid {
		conn.Close()
		return nil, errors.New("enrollment invalid")
	}

	return &Extension{
		ServerAddress: serverAddress,
		EnrollSecret:  enrollSecret,
		NodeKey:       key,
		serviceClient: client,
		clientConn:    conn,
	}, nil
}

func (e *Extension) GenerateConfigs(ctx context.Context) (map[string]string, error) {
	// TODO get version
	config, invalid, err := e.serviceClient.RequestConfig(ctx, e.NodeKey, "foobar")
	if err != nil {
		return nil, errors.Wrap(err, "transport error retrieving config")
	}

	if invalid {
		return nil, errors.New("enrollment invalid")
	}

	fmt.Println("Got config: ", config)

	return map[string]string{"config": config}, nil
}

func (e *Extension) LogString(ctx context.Context, typ logger.LogType, logText string) error {
	// TODO get version
	_, _, invalid, err := e.serviceClient.PublishLogs(ctx, e.NodeKey, "foobar", service.LogType(typ), []service.Log{service.Log{logText}})
	if err != nil {
		return errors.Wrap(err, "transport error sending logs")
	}

	if invalid {
		return errors.New("enrollment invalid")
	}

	return nil
}
