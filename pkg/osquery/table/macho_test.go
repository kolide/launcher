package table

import (
	"debug/macho"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestGenerateMacho(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents []byte
		wantCpus []string
	}{
		{
			name:     "thin binary",
			contents: thinMachoHeader(macho.CpuArm64),
			wantCpus: []string{"CpuArm64"},
		},
		{
			name:     "universal binary",
			contents: fatMachoFile(macho.CpuAmd64, macho.CpuArm64),
			wantCpus: []string{"CpuAmd64", "CpuArm64"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "Example.app", "Contents", "MacOS", "example")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
			require.NoError(t, os.WriteFile(path, tt.contents, 0600))

			results, err := generateMacho(t.Context(), tablehelpers.MockQueryContext(map[string][]string{
				"path": {path},
			}))
			require.NoError(t, err)
			require.Len(t, results, len(tt.wantCpus))

			cpus := make([]string, 0, len(results))
			for _, result := range results {
				require.Equal(t, path, result["path"])
				require.Equal(t, "Example.app", result["name"])
				cpus = append(cpus, result["cpu"])
			}
			require.ElementsMatch(t, tt.wantCpus, cpus)
		})
	}
}

func thinMachoHeader(cpu macho.Cpu) []byte {
	const macho64HeaderSize = 32

	header := make([]byte, macho64HeaderSize)
	binary.LittleEndian.PutUint32(header[0:4], macho.Magic64)
	binary.LittleEndian.PutUint32(header[4:8], uint32(cpu))
	binary.LittleEndian.PutUint32(header[12:16], uint32(macho.TypeExec))

	return header
}

func fatMachoFile(cpus ...macho.Cpu) []byte {
	const (
		fatHeaderSize  = 8
		fatArchSize    = 20
		machoHeaderLen = 32
	)

	firstMachoOffset := fatHeaderSize + len(cpus)*fatArchSize
	contents := make([]byte, firstMachoOffset+len(cpus)*machoHeaderLen)
	binary.BigEndian.PutUint32(contents[0:4], macho.MagicFat)
	binary.BigEndian.PutUint32(contents[4:8], uint32(len(cpus)))

	for i, cpu := range cpus {
		archOffset := fatHeaderSize + i*fatArchSize
		machoOffset := firstMachoOffset + i*machoHeaderLen
		binary.BigEndian.PutUint32(contents[archOffset:archOffset+4], uint32(cpu))
		binary.BigEndian.PutUint32(contents[archOffset+8:archOffset+12], uint32(machoOffset))
		binary.BigEndian.PutUint32(contents[archOffset+12:archOffset+16], machoHeaderLen)
		copy(contents[machoOffset:], thinMachoHeader(cpu))
	}

	return contents
}
