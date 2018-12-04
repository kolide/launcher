package packagekit

import (
	"fmt"
	"io"

	"github.com/groob/plist"
	"github.com/pkg/errors"
)

// Note: I wanted to just include the InitOptions struct here, but it
// did not behave as expected. plist.Encode printed the fields without
// hinting
type launchdOptions struct {
	Environment       map[string]string      `plist:"EnvironmentVariables"`
	Args              []string               `plist:"ProgramArguments"`
	Label             string                 `plist:"Label"`
	ThrottleInterval  int                    `plist:"ThrottleInterval"`
	StandardErrorPath string                 `plist:"StandardErrorPath"`
	StandardOutPath   string                 `plist:"StandardOutPath"`
	KeepAlive         map[string]interface{} `plist:"KeepAlive"`
}

type lOption func(*launchdOptions)

func RenderLaunchd(w io.Writer, initOptions *InitOptions, opts ...lOption) error {
	if initOptions.Identifier == "" {
		return errors.New("Identifier must not be empty")
	}

	if initOptions.Path == "" {
		return errors.New("Path must not be empty")
	}

	pathState := map[string]bool{
		fmt.Sprintf("/etc/%s/secret", initOptions.Identifier): true,
	}

	keepAlive := map[string]interface{}{
		"PathState": pathState,
	}

	lOpts := &launchdOptions{
		Environment:       initOptions.Environment,
		Args:              append([]string{initOptions.Path}, initOptions.Flags...),
		Label:             fmt.Sprintf("com.%s.launcher", initOptions.Identifier),
		ThrottleInterval:  60,
		StandardErrorPath: fmt.Sprintf("/var/log/%s/launcher-stderr.log", initOptions.Identifier),
		StandardOutPath:   fmt.Sprintf("/var/log/%s/launcher-stdout.log", initOptions.Identifier),
		KeepAlive:         keepAlive,
	}

	for _, opt := range opts {
		opt(lOpts)
	}

	enc := plist.NewEncoder(w)
	if err := enc.Encode(lOpts); err != nil {
		return errors.Wrap(err, "plist encode")
	}
	return nil
}
