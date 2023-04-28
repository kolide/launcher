package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		icon        menuIcon
		expectedErr bool
	}{
		{
			name:        "invalid",
			icon:        "invalid",
			expectedErr: true,
		},
		{
			name: "Translucent",
			icon: TranslucentIcon,
		},
		{
			name: "Default",
			icon: DefaultIcon,
		},
		{
			name: "TriangleExclamation",
			icon: TriangleExclamationIcon,
		},
		{
			name: "CircleX",
			icon: CircleXIcon,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			icon := getIcon(tt.icon)
			if tt.expectedErr {
				assert.Nil(t, icon)
			} else {
				assert.NotNil(t, icon)
			}
		})
	}
}
