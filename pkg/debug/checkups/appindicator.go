package checkups

import (
	"context"
	"io"
	"runtime"

	"github.com/kolide/launcher/ee/desktop/runner"
)

type appindicator struct {
	status  Status
	summary string
}

func (a *appindicator) Name() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	return "Appindicator"
}

func (a *appindicator) ExtraFileName() string {
	return ""
}

func (a *appindicator) Run(ctx context.Context, extraWriter io.Writer) error {
	if !runner.IsAppindicatorEnabled(ctx) {
		a.status = Failing
		a.summary = "No appindicator is enabled, cannot display desktop"
		return nil
	}

	a.status = Passing
	a.summary = "Appindicator enabled, can display desktop"
	return nil
}

func (a *appindicator) Status() Status {
	return a.status
}

func (a *appindicator) Summary() string {
	return a.summary
}

func (a *appindicator) Data() any {
	return nil
}
