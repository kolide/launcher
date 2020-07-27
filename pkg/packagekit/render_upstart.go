package packagekit

import (
	"context"
	"html/template"
	"io"
	"strings"

	"github.com/kolide/launcher/pkg/packagekit/internal"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// upstartOptions contains upstart specific options that are passed to
// the rendering template.
type upstartOptions struct {
	PreStartScript  []string
	PostStartScript []string
	PreStopScript   []string
	Expect          string
	Flavor          string
	StartOn         string
	StopOn          string
	ConsoleLog      bool
}

type UpstartOption func(*upstartOptions)

// WithExpect sets the expect option. This is how upstart tracks the
// pid of the daemon. See http://upstart.ubuntu.com/cookbook/#expect
func WithExpect(s string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.Expect = s
	}
}

func WithPreStartScript(s []string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.PreStartScript = s
	}
}

func WithPostStartScript(s []string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.PostStartScript = s
	}
}

func WithPreStopScript(s []string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.PreStopScript = s
	}
}

func WithUpstartFlavor(s string) UpstartOption {
	return func(uo *upstartOptions) {
		uo.Flavor = s

		switch s {
		case "amazon-ami":
			uo.StartOn = "(runlevel [345] and started network)"
			uo.StopOn = "(runlevel [!345] or stopping network)"
			uo.ConsoleLog = false
		default:
			// Also includes "" and "ubuntu":
			uo.StartOn = "net-device-up"
			uo.StopOn = "shutdown"
			uo.ConsoleLog = true
		}

	}
}

func RenderUpstart(ctx context.Context, w io.Writer, initOptions *InitOptions, uOpts ...UpstartOption) error {
	_, span := trace.StartSpan(ctx, "packagekit.Upstart")
	defer span.End()

	uOptions := &upstartOptions{}
	for _, uOpt := range uOpts {
		uOpt(uOptions)
	}

	// Prepend a "" so that the merged output looks a bit cleaner in the rendered templates
	if len(initOptions.Flags) > 0 {
		initOptions.Flags = append([]string{""}, initOptions.Flags...)
	}

	upstartTemplate, err := internal.Asset("internal/assets/upstart.sh")
	if err != nil {
		return errors.Wrapf(err, "Failed to get template named %s", "internal/assets/upstart.sh")
	}

	var data = struct {
		Common InitOptions
		Opts   upstartOptions
	}{
		Common: *initOptions,
		Opts:   *uOptions,
	}

	funcsMap := template.FuncMap{
		"StringsJoin": strings.Join,
	}

	t, err := template.New("UpstartConf").Funcs(funcsMap).Parse(string(upstartTemplate))
	if err != nil {
		return errors.Wrap(err, "not able to parse Upstart template")
	}
	return t.ExecuteTemplate(w, "UpstartConf", data)
}
