package simpleclient

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

// binaryPathInTarball returns the path to the executable within the tarball for the given platform and binary.
func binaryPathInTarball(platform, binary string) string {
	switch platform {
	case "darwin":
		if binary == "launcher" {
			return "Kolide.app/Contents/MacOS/launcher"
		}
		return "osquery.app/Contents/MacOS/osqueryd"
	case "windows":
		return binary + ".exe"
	default:
		return binary
	}
}

// extractBinaryFromTarGz extracts the binary from the tarball bytes and returns it.
func extractBinaryFromTarGz(tarGzBytes []byte, binaryPath string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(tarGzBytes))
	if err != nil {
		return nil, fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		name := strings.TrimPrefix(header.Name, "./")
		if name == binaryPath || strings.HasSuffix(name, "/"+binaryPath) {
			return io.ReadAll(tr)
		}
	}

	return nil, fmt.Errorf("binary %s not found in tarball", binaryPath)
}
