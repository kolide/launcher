package secretscan

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateArgon2idHash_Success(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		secret string
		salt   string
		hash   string
	}{
		{
			name:   "secret1, salt1",
			secret: "my-secret-api-key",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "53a76f",
		},
		{
			name:   "secret2, salt1",
			secret: "some totally different secret",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "48c55e",
		},
		{
			name:   "empty secret, salt1",
			secret: "",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "711773",
		},

		{
			name:   "secret1, salt2",
			secret: "my-secret-api-key",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "1e092d",
		},
		{
			name:   "secret2, salt2",
			secret: "some totally different secret",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "a9875b",
		},
		{
			name:   "empty secret, salt2",
			secret: "",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "55432d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hash, err := generateArgon2idHash(tt.secret, tt.salt)
			require.NoError(t, err)
			require.Equal(t, tt.hash, hash)
		})
	}
}

func TestGenerateArgon2idHash_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		secret  string
		salt    string
		wantErr string
	}{
		{
			name:    "empty salt",
			secret:  "secret",
			salt:    "",
			wantErr: "must provide a salt",
		},
		{
			name:    "invalid base64 salt",
			secret:  "secret",
			salt:    "not-valid-base64!!!",
			wantErr: "decoding salt:",
		},
		{
			name:    "salt too short",
			secret:  "secret",
			salt:    base64.StdEncoding.EncodeToString(make([]byte, 8)),
			wantErr: "salt should be 16 bytes, but is 8",
		},
		{
			name:    "salt too long",
			secret:  "secret",
			salt:    base64.StdEncoding.EncodeToString(make([]byte, 32)),
			wantErr: "salt should be 16 bytes, but is 32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hash, err := generateArgon2idHash(tt.secret, tt.salt)
			require.ErrorContains(t, err, tt.wantErr)
			require.Empty(t, hash, "hash should be empty on error")
		})
	}
}
