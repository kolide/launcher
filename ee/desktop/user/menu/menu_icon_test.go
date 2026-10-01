package menu

import (
	"testing"

	"github.com/stretchr/testify/require"
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kolideIcon := getIcon(tt.icon, false)
			require.NotEmpty(t, kolideIcon)

			deviceTrustIcon := getIcon(tt.icon, true)
			require.NotEmpty(t, deviceTrustIcon)

			require.NotEqual(t, kolideIcon, deviceTrustIcon, "device trust rebrand should select a different icon")
		})
	}
}
