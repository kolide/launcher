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
			hash:   "sOUjUs9yGqs+SkRuigBXSfmhERJcCGJ3fNnut4t7",
		},
		{
			name:   "secret2, salt1",
			secret: "some totally different secret",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "id+jdwdqmfZWwbgwOxJGbTW2+djRRwXlNl2e9zfv",
		},
		{
			name:   "empty secret, salt1",
			secret: "",
			salt:   "1QC9NyP0IOpCi99ZRGydWQ==",
			hash:   "LbzSB9Safa9Xw0YGGQFyvlG9zKHj4NCQqmuNpk5t",
		},

		{
			name:   "secret1, salt2",
			secret: "my-secret-api-key",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "PsvSqF5BqpQ6PqQ6Fgxg2iq2TwYHZdY2Pwn+AelW",
		},
		{
			name:   "secret2, salt2",
			secret: "some totally different secret",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "oMXzFhcBZlXGEbkPtHL12mU5MrQZrt1Lgn66M2jh",
		},
		{
			name:   "empty secret, salt2",
			secret: "",
			salt:   "iY4GILUmzsaZh2yPVDRcfg==",
			hash:   "0oOa06bKdi+2enIhIPVJ2bi3l4I6phvaxN0+8F5e",
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
