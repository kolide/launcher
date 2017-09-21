package packaging

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/kolide/kit/env"
	"github.com/pkg/errors"
)

// Gopath will return the current GOPATH as set by environment variables and
// will fall back to ~/go if a GOPATH is not set.
func Gopath() string {
	home := env.String("HOME", "~/")
	return env.String("GOPATH", filepath.Join(home, "go"))
}

// LauncherSource returns the path of the launcher codebase, based on the
// current GOPATH
func LauncherSource() string {
	return filepath.Join(Gopath(), "/src/github.com/kolide/launcher")
}

const (
	// DirMode is the default permission used when creating directories
	DirMode = 0755
	// DirMode is the default permission used when creating files
	FileMode = 0644
)

// CopyDir is a utility to assist with copying a directory from src to dest.
// Note that directory permissions are not maintained, but the permissions of
// the files in those directories are.
func CopyDir(src, dest string) error {
	dir, err := os.Open(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, DirMode); err != nil {
		return err
	}

	files, err := dir.Readdir(-1)
	if err != nil {
		return err
	}
	for _, file := range files {
		srcptr := filepath.Join(src, file.Name())
		dstptr := filepath.Join(dest, file.Name())
		if file.IsDir() {
			if err := CopyDir(srcptr, dstptr); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcptr, dstptr); err != nil {
				return err
			}
		}
	}
	return nil
}

// CopyFile is a utility to assist with copying a file from src to dest.
// Note that file permissions are maintained.
func CopyFile(src, dest string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destfile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destfile.Close()

	_, err = io.Copy(destfile, source)
	if err != nil {
		return err
	}
	sourceinfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dest, sourceinfo.Mode())
}

// UntarDownload will untar a source tar.gz archive to the supplied destination
func UntarDownload(destination string, source string) error {
	f, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "open download source")
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrapf(err, "create gzip reader from %s", source)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "reading tar file")
		}

		path := filepath.Join(filepath.Dir(destination), header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return errors.Wrapf(err, "creating directory for tar file: %s", path)
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return errors.Wrapf(err, "open file %s", path)
		}
		defer file.Close()
		if _, err := io.Copy(file, tr); err != nil {
			return errors.Wrapf(err, "copy tar %s to destination %s", header.FileInfo().Name(), path)
		}
	}
	return nil
}
