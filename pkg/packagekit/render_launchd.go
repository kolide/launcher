package packagekit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/groob/plist"

	"go.opencensus.io/trace"
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
	RunAtLoad         bool                   `plist:"RunAtLoad"`
}

func RenderLaunchd(ctx context.Context, w io.Writer, initOptions *InitOptions) error {
	_, span := trace.StartSpan(ctx, "packagekit.RenderLaunchd")
	defer span.End()

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
		StandardErrorPath: filepath.Join("/var/log", initOptions.Identifier, "launcher-stderr.log"),
		StandardOutPath:   filepath.Join("/var/log", initOptions.Identifier, "launcher-stdout.log"),
		KeepAlive:         keepAlive,
		RunAtLoad:         true,
	}

	enc := plist.NewEncoder(w)
	enc.Indent("   ")
	if err := enc.Encode(lOpts); err != nil {
		return fmt.Errorf("plist encode: %w", err)
	}
	return nil
}
