package checkups

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"golang.org/x/exp/maps"
)

// recursiveDirectoryContents descends through a directory, writing the files found to the provided `extraFH`.
// It returns the count of files at the top level, and any errors. This can be used to answer checkup
// questions like does this dir have at least 3 files, and list the full contents as extra.
func recursiveDirectoryContents(extraFH io.Writer, basedir string) (int, error) {
	// I'm not sure why, but WalkDir seems to visit some things twice. Meanwhile, just use a map to to count things
	files := make(map[string]bool, 0)

	// Handle occasional trailing slash on base directory
	basedir = filepath.Clean(basedir)

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

// addFileToZip takes a file path, and a zip writer, and adds the file and some metadata.
func addFileToZip(z *zip.Writer, location string) error {
	// Create metadata file first, keeping existing pattern
	metaout, err := z.Create(filepath.Join(".", location+".flaremeta"))
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", location+".flaremeta", err)
	}

	// Get file info
	fi, err := os.Stat(location)
	if os.IsNotExist(err) || os.IsPermission(err) {
		fmt.Fprintf(metaout, `{ "error stating file": "%s" }`, err)
		return nil
	}

	// Marshal metadata
	b, err := json.Marshal(fileInfo{
		Name:    fi.Name(),
		Size:    fi.Size(),
		Mode:    fi.Mode().String(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
	})
	if err != nil {
		// Structural error. Abort
		return fmt.Errorf("marshalling json: %w", err)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		// Structural error. Abort
		return fmt.Errorf("indenting json: %w", err)
	}

	if _, err := metaout.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	//
	// Done with metadata, and we know the file exists, and that we have permission to it.
	//

	fh, err := os.Open(location)
	if err != nil {
		fmt.Fprintf(metaout, `{ "error opening file": "%s" }`, err)
		return nil
	}
	defer fh.Close()

	// Create zip header with metadata
	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return fmt.Errorf("creating file header: %w", err)
	}
	header.Name = filepath.Join(".", location)

	// Create file in zip with metadata
	dataout, err := z.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", location, err)
	}

	if _, err := io.Copy(dataout, fh); err != nil {
		return fmt.Errorf("copy data into zip file %s: %w", location, err)
	}

	return nil
}

func addStreamToZip(z *zip.Writer, name string, modTime time.Time, reader io.Reader) error {
	// Create metadata file first
	metaout, err := z.Create(name + ".flaremeta")
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", name+".flaremeta", err)
	}

	// Marshal metadata
	b, err := json.Marshal(fileInfo{
		Name:    filepath.Base(name),
		ModTime: modTime,
	})
	if err != nil {
		return fmt.Errorf("marshalling json: %w", err)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return fmt.Errorf("indenting json: %w", err)
	}

	if _, err := metaout.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	// Create the main file in zip
	header := &zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: modTime,
	}

	out, err := z.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", name, err)
	}

	if _, err := io.Copy(out, reader); err != nil {
		return fmt.Errorf("copying data to zip: %w", err)
	}

	return nil
}

var ignoredEnvPrefixes = []string{
	"LESS",
	"LS_COLORS",
	"SECURITYSESSIONID",
	"SSH",
	"TERM_SESSION_ID",
}

// runCmdMarkdownLogged is a wrapper over cmd.Run that does some output formatting. Callers are expected to have
// created the cmd with appropriate environment and io writers.
func runCmdMarkdownLogged(cmd *exec.Cmd, extraWriter io.Writer) error {
	if extraWriter != io.Discard {
		fmt.Fprintf(extraWriter, "```shell\n")
		fmt.Fprintf(extraWriter, "# ENV:\n")
		for _, e := range cmd.Environ() {
			skip := false
			for _, prefix := range ignoredEnvPrefixes {
				if strings.HasPrefix(e, prefix) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			fmt.Fprintf(extraWriter, "export %s\n", e)
		}
		fmt.Fprintf(extraWriter, "\n$ %s\n", cmd.String())

		defer fmt.Fprintf(extraWriter, "```\n\n")

		if cmd.Stderr == nil {
			cmd.Stderr = extraWriter
		} else {
			io.MultiWriter(extraWriter, cmd.Stderr)
		}

		if cmd.Stdout == nil {
			cmd.Stdout = extraWriter
		} else {
			cmd.Stdout = io.MultiWriter(extraWriter, cmd.Stdout)
		}
	}

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(extraWriter, "\n\n# Got Error: %s\n", err)
		return err
	}

	return nil
}

func parseUrl(k types.Knapsack, addr string) (*url.URL, error) {
	if !strings.HasPrefix(addr, "http") {
		scheme := "https"
		if k.InsecureTransportTLS() {
			scheme = "http"
		}
		addr = fmt.Sprintf("%s://%s", scheme, addr)
	}

	u, err := url.Parse(addr)

	if err != nil {
		return nil, err
	}

	if u.Port() == "" {
		port := "443"
		if k.InsecureTransportTLS() {
			port = "80"
		}
		u.Host = net.JoinHostPort(u.Host, port)
	}

	return u, nil
}
