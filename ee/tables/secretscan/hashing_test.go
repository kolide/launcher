package secretscan

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateArgon2idHash_Success(t *testing.T) {
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
			hash:   "YTI+9SPdgNCnNvSNMnmC",
		},
		{
			name:   "secret2, salt1",
			secret: "some totally different secret",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "1e9hrG2HJlmwbfdgn8ud",
		},
		{
			name:   "empty secret, salt1",
			secret: "",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "jQMqX6DSoQcOBNMxVkRl",
		},

		{
			name:   "secret1, salt2",
			secret: "my-secret-api-key",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "fwhf0he8bPyWt9Jm0mQJ",
		},
		{
			name:   "secret2, salt2",
			secret: "some totally different secret",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "JAuPi57HAnFOrrNniQvx",
		},
		{
			name:   "empty secret, salt2",
			secret: "",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "tNivLjqEO9FPHASI70+t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := generateArgon2idHash(tt.secret, tt.salt)
			require.NoError(t, err)
			require.Equal(t, tt.hash, hash)
		})
	}
}

func TestGenerateArgon2idHash_Error(t *testing.T) {
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
			hash, err := generateArgon2idHash(tt.secret, tt.salt)
			require.ErrorContains(t, err, tt.wantErr)
			require.Empty(t, hash, "hash should be empty on error")
		})
	}
}
