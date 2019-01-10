package packagekit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateMicrosoftProductCode(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		ident1 string
		identN []string
		out    string
	}{
		{
			ident1: "launcher",
			identN: []string{},
			out:    "F3E08B51-1935-8A8F-58F1-7A678759F60C",
		},
		{
			ident1: "launcher",
			identN: []string{"kolide-app"},
			out:    "3367A041-D1DA-D2D4-5A6C-E7A286F024C5",
		},
		{
			ident1: "launcher",
			identN: []string{"kolide-app", "0.7.0"},
			out:    "05CFB3A6-0882-AF2A-B11B-E0E07589C1D1",
		},
	}

	for _, tt := range tests {
		guid := generateMicrosoftProductCode(tt.ident1, tt.identN...)
		require.Equal(t, len("XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"), len(guid))
		require.Equal(t, tt.out, guid)
	}

}
