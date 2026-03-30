package indexeddb

import (
	"testing"

	"github.com/golang/snappy"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_deserializeIndexeddbValue(t *testing.T) {
	t.Parallel()

	testBytes := []byte{
		// header
		0x04, // padding, ignore
		0xff, // version tag
		0x01, // version
		0x00, // padding, ignore
		0x00, // padding, ignore
		0x00, // padding, ignore
		0xff, // version tag (indicates end of header)
		0x0f, // version
		// object
		0x6f, // object begin
		0x22, // string tag
		0x02, // string length of 2
		0x69, // i
		0x64, // d
		0x49, // int32 tag
		0x02, // value for ID
		0x7b, // object end
		0x01, // properties_written
	}

	objs, err := DeserializeChrome(t.Context(), multislogger.NewNopLogger(), map[string][]byte{"data": testBytes})
	require.NoError(t, err, "deserializing object")
	require.Len(t, objs, 1)

	// Confirm we got an id property for the object
	require.Contains(t, objs[0], "id", "expected id property")
}

func Test_deserializeIndexeddbValue_InvalidType(t *testing.T) {
	t.Parallel()

	testBytes := []byte{
		// header
		0x04, // padding, ignore
		0xff, // version tag
		0x01, // version
		0x00, // padding, ignore
		0x00, // padding, ignore
		0x00, // padding, ignore
		0xff, // version tag (indicates end of header)
		0x0f, // version
		// object
		0x6f, // object begin
		0x54, // boolean true tag -- invalid data!
		0x7b, // object end
		0x00, // properties_written
	}

	_, err := DeserializeChrome(t.Context(), multislogger.NewNopLogger(), map[string][]byte{"data": testBytes})
	require.Error(t, err, "should not have been able to deserialize malformed object")
}

func Test_snappyDecompressedIfNeeded(t *testing.T) {
	t.Parallel()

	decompressedInner := []byte("hello indexeddb")
	validWrapped := append(
		[]byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy},
		snappy.Encode(nil, decompressedInner)...,
	)

	tests := []struct {
		name      string
		payload   []byte
		want      []byte
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "nil returns unchanged",
			payload: nil,
			want:    nil,
		},
		{
			name:    "empty return unchanged",
			payload: []byte{},
			want:    []byte{},
		},
		{
			name:    "too short header returns unchanged",
			payload: []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion},
			want:    []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion},
		},
		{
			name:    "prefix only does not attempt decompression",
			payload: []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy},
			want:    []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy},
		},
		{
			name:    "unchanged without all token prefix bytes matching",
			payload: []byte{0xfe, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy, 0x00},
			want:    []byte{0xfe, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy, 0x00},
		},
		{
			name:    "arbitrary payload without magic prefix returns unchanged",
			payload: []byte{0x6f, 0x22, 0x02, 0x61, 0x62},
			want:    []byte{0x6f, 0x22, 0x02, 0x61, 0x62},
		},
		{
			name:    "valid snappy wrapper decompresses payload correctly",
			payload: validWrapped,
			want:    decompressedInner,
		},
		{
			name: "invalid snappy compression returns error",
			payload: append(
				[]byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy},
				0x00, 0x01, 0x02, 0x03,
			),
			wantErr:   true,
			errSubstr: "snappy decompress after Chrome FF/11/02 wrapper",
		},
		{
			name: "empty snappy data returns error",
			payload: append(
				[]byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy},
				snappy.Encode(nil, []byte{})...,
			),
			wantErr:   true,
			errSubstr: "snappy decompression yielded empty data set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := snappyDecompressedIfNeeded(tt.payload)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					require.ErrorContains(t, err, tt.errSubstr)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
