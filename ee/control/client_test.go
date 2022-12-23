package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// TestClient is useful for clients in tests
type TestClient struct {
	subsystemMap map[string]string
	hashData     map[string]any
}

func NewControlTestClient(subsystemMap map[string]string, hashData map[string]any) (*TestClient, error) {
	c := &TestClient{
		subsystemMap: subsystemMap,
		hashData:     hashData,
	}
	return c, nil
}

func (c *TestClient) Get(hash string) (data io.Reader, err error) {
	if hash == "" {
		bodyBytes, err := json.Marshal(c.subsystemMap)
		if err != nil {
			return nil, fmt.Errorf("marshaling json: %w", err)
		}

		return bytes.NewReader(bodyBytes), nil
	}

	bodyBytes, err := json.Marshal(c.hashData[hash])
	if err != nil {
		return nil, fmt.Errorf("marshaling json: %w", err)
	}

	return bytes.NewReader(bodyBytes), nil
}
