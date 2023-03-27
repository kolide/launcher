package flags

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
)

func assertGet(t *testing.T, fc *FlagController, key FlagKey, expectedValue any) {
	var actualValue any
	switch expectedValue.(type) {
	case bool:
		actualValue = get[bool](fc, key)
	case string:
		actualValue = get[string](fc, key)
	case int64:
		actualValue = get[int64](fc, key)
	}
	assert.Equal(t, expectedValue, actualValue)
}

func TestFlagControllerNoValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "empty",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fc := NewFlagController(log.NewNopLogger(), nil, nil, nil, nil)
			assertGet(t, fc, ControlRequestInterval, int64(0))
		})
	}
}
