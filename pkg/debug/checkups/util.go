package checkups

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/exp/maps"
)

// recursiveDirectoryContents descends through a directory, writing the files found to the provided `extraFH`.
// It returns the count of files at the top level, and any errors. This can be used to answer checkup
// questions like does this dir have at least 3 files, and list the full contents as extra.
func recursiveDirectoryContents(extraFH io.Writer, basedir string) (int, error) {
	// I'm not sure why, but WalkDir seems to visit some things twice. Meanwhile, just use a map to to count things
	files := make(map[string]bool, 0)

	filewalkErr := filepath.WalkDir(basedir, func(path string, d fs.DirEntry, err error) error {
		if filepath.Dir(path) == basedir {
			files[path] = true
		}

		fmt.Fprintln(extraFH, path)
		return nil
	})

	return len(maps.Keys(files)), filewalkErr

}

type fileInfo struct {
	Name    string    // base name of the file
	Size    int64     // length in bytes for regular files; system-dependent for others
	Mode    string    // file mode bits
	ModTime time.Time // modification time
	IsDir   bool      // abbreviation for Mode().IsDir()
}

func addFileToZip(z *zip.Writer, location string) error {
	metaout, err := z.Create(filepath.Join(".", location+".flaremeta"))
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", location+".flaremeta", err)
	}

	// Not totally clear if we should use Lstat or Stat here.
	fi, err := os.Stat(location)
	if os.IsNotExist(err) || os.IsPermission(err) {
		fmt.Fprintf(metaout, `{ "error stating file": "%s" }\n`, err)
		return nil
	}

	b, err := json.Marshal(fileInfo{
		Name:    fi.Name(),
		Size:    fi.Size(),
		Mode:    fi.Mode().String(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
	})
	if err != nil {
		// Structural error. Abort
		return fmt.Errorf("marhsaling json: %w", err)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		// Structural error. Abort
		return fmt.Errorf("indenting json: %w", err)

	}
	metaout.Write(buf.Bytes())

	//
	// Done with metadata, and we know the file exists, and that we have permission to it.
	//

	fh, err := os.Open(location)
	if err != nil {
		fmt.Fprintf(metaout, `{ "error opening file": "%s" }\n`, err)
		return nil
	}
	defer fh.Close()

	//var errToLog
	dataout, err := z.Create(filepath.Join(".", location))
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", location, err)
	}

	if _, err := io.Copy(dataout, fh); err != nil {
		return fmt.Errorf("copy data into zip file %s: %w", location, err)
	}
	return nil

}
