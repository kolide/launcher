package checkups

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Logs struct {
	Filepaths []string
}

func (c *Logs) Name() string {
	return "Check logs"
}

func (c *Logs) Run(short io.Writer) (string, error) {
	return checkupLogFiles(short, c.Filepaths)
}

// checkupLogFiles checks to see if expected log files are present
func checkupLogFiles(short io.Writer, filepaths []string) (string, error) {
	var foundCurrentLogFile bool
	for _, f := range filepaths {
		filename := filepath.Base(f)
		info(short, filename)

		if filename != "debug.json" {
			continue
		}

		foundCurrentLogFile = true

		fi, err := os.Stat(f)
		if err != nil {
			continue
		}

		info(short, "")
		info(short, fmt.Sprintf("Most recent log file:\t%s", filename))
		info(short, fmt.Sprintf("Latest modification:\t%s", fi.ModTime().String()))
		info(short, fmt.Sprintf("File size (B):\t%d", fi.Size()))
	}

	if !foundCurrentLogFile {
		return "", fmt.Errorf("No log file found")
	}

	return "Log file found", nil
}
