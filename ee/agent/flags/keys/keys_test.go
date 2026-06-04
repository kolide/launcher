package keys

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		flagKeys []FlagKey
		key      FlagKey
		want     bool
	}{
		{
			name:     "nil slice",
			flagKeys: nil,
			key:      Debug,
			want:     false,
		},
		{
			name:     "empty slice",
			flagKeys: []FlagKey{},
			key:      Debug,
			want:     false,
		},
		{
			name:     "single match",
			flagKeys: []FlagKey{Debug},
			key:      Debug,
			want:     true,
		},
		{
			name:     "single no match",
			flagKeys: []FlagKey{Debug},
			key:      Autoupdate,
			want:     false,
		},
		{
			name:     "match first",
			flagKeys: []FlagKey{Debug, Autoupdate, KolideServerURL},
			key:      Debug,
			want:     true,
		},
		{
			name:     "match last",
			flagKeys: []FlagKey{Debug, Autoupdate, KolideServerURL},
			key:      KolideServerURL,
			want:     true,
		},
		{
			name:     "no match among several",
			flagKeys: []FlagKey{Debug, Autoupdate},
			key:      ControlServerURL,
			want:     false,
		},
		{
			name:     "duplicates still found",
			flagKeys: []FlagKey{Debug, Debug, Autoupdate},
			key:      Debug,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Contains(tt.flagKeys, tt.key)
			require.Equal(t, tt.want, got)
		})
	}
}
