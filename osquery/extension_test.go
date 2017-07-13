package osquery

import (
	"context"
	"errors"
	"testing"

	"github.com/kolide/launcher/service/mock"
	"github.com/kolide/osquery-go/plugin/distributed"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionEnrollTransportError(t *testing.T) {
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return "", false, errors.New("transport")
		},
	}
	e := &Extension{serviceClient: m}

	key, invalid, err := e.Enroll(context.Background(), "foo", "bar")
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnrollSecretInvalid(t *testing.T) {
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			return "", true, nil
		},
	}
	e := &Extension{serviceClient: m}

	key, invalid, err := e.Enroll(context.Background(), "foo", "bar")
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	assert.Equal(t, "", key)
	assert.True(t, invalid)
	assert.NotNil(t, err)
}

func TestExtensionEnroll(t *testing.T) {
	var gotEnrollSecret, gotHostIdentifier string
	expectedNodeKey := "node_key"
	m := &mock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string) (string, bool, error) {
			gotEnrollSecret = enrollSecret
			gotHostIdentifier = hostIdentifier
			return expectedNodeKey, false, nil
		},
	}
	e := &Extension{serviceClient: m}

	expectedEnrollSecret := "foo_secret"
	expectedHostIdentifier := "bar_host"
	key, invalid, err := e.Enroll(context.Background(), expectedEnrollSecret, expectedHostIdentifier)
	assert.True(t, m.RequestEnrollmentFuncInvoked)
	require.Nil(t, err)
	assert.False(t, invalid)
	assert.Equal(t, expectedNodeKey, key)
	assert.Equal(t, expectedEnrollSecret, gotEnrollSecret)
	assert.Equal(t, expectedHostIdentifier, gotHostIdentifier)
}

func TestExtensionGenerateConfigsTransportError(t *testing.T) {
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string, version string) (string, bool, error) {
			return "", false, errors.New("transport")
		},
	}
	e := &Extension{serviceClient: m}

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Nil(t, configs)
	assert.NotNil(t, err)
}

func TestExtensionGenerateConfigsEnrollmentInvalid(t *testing.T) {
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string, version string) (string, bool, error) {
			return "", true, nil
		},
	}
	e := &Extension{serviceClient: m}

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Nil(t, configs)
	assert.NotNil(t, err)
}

func TestExtensionGenerateConfigs(t *testing.T) {
	configVal := `{"foo": "bar"}`
	m := &mock.KolideService{
		RequestConfigFunc: func(ctx context.Context, nodeKey string, version string) (string, bool, error) {
			return configVal, false, nil
		},
	}
	e := &Extension{serviceClient: m}

	configs, err := e.GenerateConfigs(context.Background())
	assert.True(t, m.RequestConfigFuncInvoked)
	assert.Equal(t, map[string]string{"config": configVal}, configs)
	assert.Nil(t, err)
}

func TestExtensionLogStringTransportError(t *testing.T) {
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, version string, logType logger.LogType, logs []string) (string, string, bool, error) {
			return "", "", false, errors.New("transport")
		},
	}
	e := &Extension{serviceClient: m}

	err := e.LogString(context.Background(), logger.LogTypeSnapshot, "foobar")
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.NotNil(t, err)
}

func TestExtensionLogStringEnrollmentInvalid(t *testing.T) {
	var gotNodeKey string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, version string, logType logger.LogType, logs []string) (string, string, bool, error) {
			gotNodeKey = nodeKey
			return "", "", true, nil
		},
	}
	expectedNodeKey := "node_key"
	e := &Extension{serviceClient: m, NodeKey: expectedNodeKey}

	err := e.LogString(context.Background(), logger.LogTypeString, "foobar")
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionLogString(t *testing.T) {
	var gotNodeKey, gotVersion string
	var gotLogType logger.LogType
	var gotLogs []string
	m := &mock.KolideService{
		PublishLogsFunc: func(ctx context.Context, nodeKey string, version string, logType logger.LogType, logs []string) (string, string, bool, error) {
			gotNodeKey = nodeKey
			gotVersion = version
			gotLogType = logType
			gotLogs = logs
			return "", "", false, nil
		},
	}

	expectedNodeKey := "node_key"
	e := &Extension{serviceClient: m, NodeKey: expectedNodeKey}

	err := e.LogString(context.Background(), logger.LogTypeStatus, "foobar")
	assert.True(t, m.PublishLogsFuncInvoked)
	assert.Nil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
	assert.Equal(t, logger.LogTypeStatus, gotLogType)
	assert.Equal(t, []string{"foobar"}, gotLogs)
}

func TestExtensionGetQueriesTransportError(t *testing.T) {
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string, version string) (*distributed.GetQueriesResult, bool, error) {
			return nil, false, errors.New("transport")
		},
	}
	e := &Extension{serviceClient: m}

	queries, err := e.GetQueries(context.Background())
	assert.True(t, m.RequestQueriesFuncInvoked)
	assert.NotNil(t, err)
	assert.Nil(t, queries)
}

func TestExtensionGetQueriesEnrollmentInvalid(t *testing.T) {
	var gotNodeKey string
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string, version string) (*distributed.GetQueriesResult, bool, error) {
			gotNodeKey = nodeKey
			return nil, true, nil
		},
	}
	expectedNodeKey := "node_key"
	e := &Extension{serviceClient: m, NodeKey: expectedNodeKey}

	queries, err := e.GetQueries(context.Background())
	assert.True(t, m.RequestQueriesFuncInvoked)
	assert.NotNil(t, err)
	assert.Nil(t, queries)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionGetQueries(t *testing.T) {
	expectedQueries := map[string]string{
		"time":    "select * from time",
		"version": "select version from osquery_info",
	}
	m := &mock.KolideService{
		RequestQueriesFunc: func(ctx context.Context, nodeKey string, version string) (*distributed.GetQueriesResult, bool, error) {
			return &distributed.GetQueriesResult{
				Queries: expectedQueries,
			}, false, nil
		},
	}
	e := &Extension{serviceClient: m}

	queries, err := e.GetQueries(context.Background())
	assert.True(t, m.RequestQueriesFuncInvoked)
	require.Nil(t, err)
	assert.Equal(t, expectedQueries, queries.Queries)
}

func TestExtensionWriteResultsTransportError(t *testing.T) {
	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			return "", "", false, errors.New("transport")
		},
	}
	e := &Extension{serviceClient: m}

	err := e.WriteResults(context.Background(), []distributed.Result{})
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.NotNil(t, err)
}

func TestExtensionWriteResultsEnrollmentInvalid(t *testing.T) {
	var gotNodeKey string
	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			gotNodeKey = nodeKey
			return "", "", true, nil
		},
	}
	expectedNodeKey := "node_key"
	e := &Extension{serviceClient: m, NodeKey: expectedNodeKey}

	err := e.WriteResults(context.Background(), []distributed.Result{})
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.NotNil(t, err)
	assert.Equal(t, expectedNodeKey, gotNodeKey)
}

func TestExtensionWriteResults(t *testing.T) {
	var gotResults []distributed.Result
	m := &mock.KolideService{
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			gotResults = results
			return "", "", true, nil
		},
	}
	e := &Extension{serviceClient: m}

	expectedResults := []distributed.Result{
		{
			QueryName: "foobar",
			Status:    0,
			Rows:      []map[string]string{{"foo": "bar"}},
		},
	}

	err := e.WriteResults(context.Background(), expectedResults)
	assert.True(t, m.PublishResultsFuncInvoked)
	assert.NotNil(t, err)
	assert.Equal(t, expectedResults, gotResults)
}
