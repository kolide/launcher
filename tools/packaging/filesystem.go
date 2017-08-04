package packaging

import (
	"io"
	"os"
	"path/filepath"

	"github.com/kolide/kit/env"
)

// gopath will return the current gopath as set by environment variables and
// will fall back to ~/go if a gopath is not set.
func gopath() string {
	home := env.String("HOME", "~/")
	return env.String("GOPATH", filepath.Join(home, "go"))
}

// launcherSource returns the path of the launcher codebase, based on the
// current gopath
func LauncherSource() string {
	return filepath.Join(gopath(), "/src/github.com/kolide/launcher")
}

const (
	// DirMode is the default permission used when creating directories
	DirMode = 0755
	// DirMode is the default permission used when creating files
	FileMode = 0644
)

// copyDir is a utility to assist with copying a directory from src to dest.
// Note that directory permissions are not maintained, but the permissions of
// the files in those directories are.
func copyDir(src, dest string) error {
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
			if err := copyDir(srcptr, dstptr); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcptr, dstptr); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile is a utility to assist with copying a file from src to dest.
// Note that file permissions are maintained.
func copyFile(src, dest string) error {
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
