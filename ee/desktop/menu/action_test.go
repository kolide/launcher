package menu

import (
	_ "embed"

	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Unmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		data        string
		action      Action
		expectedErr bool
	}{
		{
			name:        "nil",
			expectedErr: true,
		},
		{
			name: "empty",
			data: `{}`,
		},
		{
			name: "do nothing",
			data: `{"type":""}`,
		},
		{
			name:        "unknown action",
			data:        `{"type":"unknown_action"}`,
			expectedErr: true,
		},
		{
			name:   "open url",
			data:   `{"type":"open-url","action":{"url":"https://localhost:3443"}}`,
			action: Action{Type: OpenURL, Action: json.RawMessage(`{"url":"https://localhost:3443"}`), Performer: actionOpenURL{URL: "https://localhost:3443"}},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var a Action
			err := json.Unmarshal([]byte(tt.data), &a)
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.action, a)

		})
	}
}
