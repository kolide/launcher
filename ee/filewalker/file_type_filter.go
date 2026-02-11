package filewalker

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// fileTypeFilter is an optional component of the filewalk configuration.
// When it is set, it allows for restricting filewalk results to only files
// or only directories.
type fileTypeFilter struct {
	name    string
	matches func(f fs.FileMode) bool
}

const (
	fileTypeFile = "file"
	fileTypeDir  = "dir"
)

func (ft *fileTypeFilter) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return fmt.Errorf("unmarshalling string: %w", err)
	}

	switch s {
	case fileTypeFile:
		ft.name = fileTypeFile
		ft.matches = func(f fs.FileMode) bool {
			return !f.IsDir()
		}
		return nil
	case fileTypeDir:
		ft.name = fileTypeDir
		ft.matches = func(f fs.FileMode) bool {
			return f.IsDir()
		}
		return nil
	default:
		return fmt.Errorf("unsupported file filter type %s", s)
	}
}
