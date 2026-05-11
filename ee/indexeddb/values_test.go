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

func Test_handleWrappedValues(t *testing.T) {
	t.Parallel()

	decompressedInner := []byte("hello indexeddb")
	validSnappyHeaderWithoutIndexeddbVersion := append([]byte{tokenVersion}, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy)
	validWrappedWithoutIndexeddbVersion := append(
		validSnappyHeaderWithoutIndexeddbVersion,
		snappy.Encode(nil, decompressedInner)...,
	)
	emptyWrappedWithoutIndexeddbVersion := append(
		validSnappyHeaderWithoutIndexeddbVersion,
		snappy.Encode(nil, []byte{})...,
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
			payload: append(uvarintToBytes(100), tokenVersion, tokenRequiresProcessingSSVPseudoVersion),
			want:    append(uvarintToBytes(100), tokenVersion, tokenRequiresProcessingSSVPseudoVersion),
		},
		{
			name:    "prefix only does not attempt decompression",
			payload: append(uvarintToBytes(200), tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy),
			want:    append(uvarintToBytes(200), tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy),
		},
		{
			name:    "unchanged without all token prefix bytes matching",
			payload: append(uvarintToBytes(300), 0xfe, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy, 0x00),
			want:    append(uvarintToBytes(300), 0xfe, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy, 0x00),
		},
		{
			name:    "arbitrary payload without magic prefix returns unchanged",
			payload: []byte{0x6f, 0x22, 0x02, 0x61, 0x62},
			want:    []byte{0x6f, 0x22, 0x02, 0x61, 0x62},
		},
		{
			name:    "valid snappy wrapper decompresses payload correctly",
			payload: append(uvarintToBytes(400), validWrappedWithoutIndexeddbVersion...),
			want: append(
				uvarintToBytes(400),
				decompressedInner...,
			),
		},
		{
			name: "invalid snappy compression returns error",
			payload: append(uvarintToBytes(500), tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy,
				0x00, 0x01, 0x02, 0x03,
			),
			wantErr:   true,
			errSubstr: "snappy decompress after Chrome FF/11/02 wrapper",
		},
		{
			name:      "empty snappy data returns error",
			payload:   append(uvarintToBytes(600), emptyWrappedWithoutIndexeddbVersion...),
			wantErr:   true,
			errSubstr: "snappy decompression yielded empty data set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := handleWrappedValues(tt.payload, nil)
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
