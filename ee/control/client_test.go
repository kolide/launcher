package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// TestClient is useful for clients in tests
type TestClient struct {
	subsystemMap      map[string]string
	hashData          map[string]any
	hashRequestCounts map[string]int
}

func NewControlTestClient(subsystemMap map[string]string, hashData map[string]any) (*TestClient, error) {
	c := &TestClient{
		subsystemMap:      subsystemMap,
		hashData:          hashData,
		hashRequestCounts: make(map[string]int),
	}
	return c, nil
}

func (c *TestClient) GetConfig(_ context.Context) (data io.Reader, err error) {
	bodyBytes, err := json.Marshal(c.subsystemMap)
	if err != nil {
		return nil, fmt.Errorf("marshaling json: %w", err)
	}

	return bytes.NewReader(bodyBytes), nil
}

func (c *TestClient) GetSubsystemData(_ context.Context, hash string) (data io.Reader, err error) {
	if _, ok := c.hashRequestCounts[hash]; !ok {
		c.hashRequestCounts[hash] = 1
	} else {
		c.hashRequestCounts[hash] += 1
	}

	bodyBytes, err := json.Marshal(c.hashData[hash])
	if err != nil {
		return nil, fmt.Errorf("marshaling json: %w", err)
	}

	return bytes.NewReader(bodyBytes), nil
}

func (c *TestClient) SendMessage(_ context.Context, method string, params interface{}) error {
	return nil
}
