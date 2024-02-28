package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		icon menuIcon
	}{
		{
			name: "invalid",
			icon: "invalid",
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
		{
			name: "CircleDot",
			icon: CircleDotIcon,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			icon := getIcon(tt.icon)
			assert.NotNil(t, icon)
		})
	}
}
