package sshtable

import (
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestKeys(t *testing.T) {
	var tests = []struct {
		in        string
		keyType   string
		keySize   string
		encrypted bool
	}{
		//{in: "seph-openssh7-dsa-1024-unenc"},
		{in: "seph-openssh7-ecdsa-256-unenc"},
		{in: "seph-openssh7-ecdsa-521-unenc"},
		{in: "seph-openssh7-ed25519-2048-unenc"},
		{in: "seph-openssh7-rsa-1024-unenc"},
		{in: "seph-openssh7-rsa-2048-unenc"},
		{in: "seph-openssh7-rsa-4096-unenc"},
	}

	tbl := tableExtension{
		logger: log.NewNopLogger(),
	}

	for _, tt := range tests {
		_, err := tbl.checkFile(filepath.Join("testdata", tt.in))
		require.NoError(t, err, tt.in)

	}
}
