package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func createTestTarGz(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Mode:     0755,
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("writing tar header for %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("writing tar content for %s: %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}

	return &buf
}

func TestExtractTarGz(t *testing.T) {
	t.Parallel()

	t.Run("single file", func(t *testing.T) {
		t.Parallel()
		destDir := t.TempDir()
		buf := createTestTarGz(t, map[string]string{
			"hello.txt": "hello world",
		})

		if err := extractTarGz(buf, destDir); err != nil {
			t.Fatalf("extractTarGz: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
		if err != nil {
			t.Fatalf("reading extracted file: %v", err)
		}
		if string(got) != "hello world" {
			t.Errorf("got %q, want %q", string(got), "hello world")
		}
	})

	t.Run("nested directories", func(t *testing.T) {
		t.Parallel()
		destDir := t.TempDir()
		buf := createTestTarGz(t, map[string]string{
			"app/bin/launcher":  "launcher-binary",
			"app/lib/helper.so": "shared-lib",
		})

		if err := extractTarGz(buf, destDir); err != nil {
			t.Fatalf("extractTarGz: %v", err)
		}

		for _, tc := range []struct {
			path    string
			content string
		}{
			{"app/bin/launcher", "launcher-binary"},
			{"app/lib/helper.so", "shared-lib"},
		} {
			got, err := os.ReadFile(filepath.Join(destDir, tc.path))
			if err != nil {
				t.Errorf("reading %s: %v", tc.path, err)
				continue
			}
			if string(got) != tc.content {
				t.Errorf("%s: got %q, want %q", tc.path, string(got), tc.content)
			}
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		t.Parallel()
		destDir := t.TempDir()
		buf := createTestTarGz(t, map[string]string{
			"../etc/passwd": "evil",
		})

		err := extractTarGz(buf, destDir)
		if err == nil {
			t.Fatal("expected error for path traversal, got nil")
		}
	})

	t.Run("preserves file mode", func(t *testing.T) {
		t.Parallel()
		destDir := t.TempDir()

		var buf bytes.Buffer
		gzw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gzw)
		if err := tw.WriteHeader(&tar.Header{
			Name:     "script.sh",
			Size:     4,
			Mode:     0700,
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte("#!/sh"))
		tw.Close()
		gzw.Close()

		if err := extractTarGz(&buf, destDir); err != nil {
			t.Fatalf("extractTarGz: %v", err)
		}

		info, err := os.Stat(filepath.Join(destDir, "script.sh"))
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0700 {
			t.Errorf("mode: got %o, want %o", info.Mode().Perm(), 0700)
		}
	})

	t.Run("empty tarball", func(t *testing.T) {
		t.Parallel()
		destDir := t.TempDir()
		buf := createTestTarGz(t, map[string]string{})

		if err := extractTarGz(buf, destDir); err != nil {
			t.Fatalf("extractTarGz: %v", err)
		}

		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatalf("reading dir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected empty dir, got %d entries", len(entries))
		}
	})
}
