package filewalker

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"time"
)

// duration is a thin wrapper around time.Duration allowing for marshalling/unmarshalling
// durations as strings, rather than the default of nanoseconds.
type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("unmarshalling duration: %w", err)
	}
	parsedDuration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parsing duration: %w", err)
	}
	*d = duration(parsedDuration)
	return nil
}

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

type (
	filewalkConfig struct {
		WalkInterval duration `json:"walk_interval"`
		filewalkDefinition
		Overlays []filewalkConfigOverlay `json:"overlays"`
	}

	filewalkConfigOverlay struct {
		Filters map[string]string `json:"filters"` // determines if this overlay is applicable to this launcher installation
		filewalkDefinition
	}

	filewalkDefinition struct {
		RootDirs       *[]string         `json:"root_dirs,omitempty"`
		FileNameRegex  *regexp.Regexp    `json:"file_name_regex,omitempty"`
		SkipDirs       *[]*regexp.Regexp `json:"skip_dirs,omitempty"`
		FileTypeFilter *fileTypeFilter   `json:"file_type_filter,omitempty"`
	}
)
