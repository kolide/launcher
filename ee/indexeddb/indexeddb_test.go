package indexeddb

import (
	"testing"

	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"
)

func Test_bodyIsWrapped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		payload           []byte
		expectedIsWrapped bool
	}{
		{
			name:              "nil",
			payload:           nil,
			expectedIsWrapped: false,
		},
		{
			name:              "empty",
			payload:           []byte{},
			expectedIsWrapped: false,
		},
		{
			name:              "too short header",
			payload:           []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion},
			expectedIsWrapped: false,
		},
		{
			name:              "prefix only",
			payload:           []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy},
			expectedIsWrapped: false,
		},
		{
			name:              "prefix does not match",
			payload:           []byte{0xfe, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy, 0x00},
			expectedIsWrapped: false,
		},
		{
			name:              "arbitrary payload does not match",
			payload:           []byte{0x6f, 0x22, 0x02, 0x61, 0x62},
			expectedIsWrapped: false,
		},
		{
			name:              "valid snappy wrapper",
			payload:           []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy, 0x00},
			expectedIsWrapped: true,
		},
		{
			name:              "valid blob wrapper",
			payload:           []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenReplaceWithBlob, 0x00},
			expectedIsWrapped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedIsWrapped, bodyIsWrapped(tt.payload))
		})
	}
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
			payload: validWrappedWithoutIndexeddbVersion,
			want:    decompressedInner,
		},
		{
			name: "invalid snappy compression returns error",
			payload: []byte{tokenVersion, tokenRequiresProcessingSSVPseudoVersion, tokenCompressedWithSnappy,
				0x00, 0x01, 0x02, 0x03,
			},
			wantErr:   true,
			errSubstr: "snappy decompress",
		},
		{
			name:      "empty snappy data returns error",
			payload:   emptyWrappedWithoutIndexeddbVersion,
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
