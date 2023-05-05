package knapsack

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/knapsack/mocks"
	"github.com/stretchr/testify/assert"
)

func TestQueuedQuerier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mock    func(t *testing.T) *mocks.Querier
		queries []string
	}{
		{
			name: "no querier",
		},
		{
			name: "empty queue",
			mock: func(t *testing.T) *mocks.Querier {
				m := mocks.NewQuerier(t)
				return m
			},
		},
		{
			name: "happy path",
			mock: func(t *testing.T) *mocks.Querier {
				m := mocks.NewQuerier(t)
				m.On("Query", "sql query #1").Return(nil, nil)
				m.On("Query", "sql query #2").Return(nil, nil)
				m.On("Query", "sql query #3").Return(nil, nil)
				return m
			},
			queries: []string{"sql query #1", "sql query #2", "sql query #3"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			qq := NewQueuedQuerier(log.NewNopLogger())
			if tt.mock != nil {
				qq.SetQuerier(tt.mock(t))
			}

			var callbackTimes int
			callback := func(result []map[string]string, err error) {
				callbackTimes = callbackTimes + 1
			}

			for _, q := range tt.queries {
				qq.Query(q, callback)
			}

			qq.processQueue()

			assert.Equal(t, len(tt.queries), callbackTimes)
			assert.Equal(t, 0, qq.queue.Len())
		})
	}
}
