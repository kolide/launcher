package packagekit

import (
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"strings"

	"go.opencensus.io/trace"
)

//go:embed assets/upstart.sh
var upstartTemplate []byte

// upstartOptions contains upstart specific options that are passed to
// the rendering template.
type upstartOptions struct {
	ConsoleLog      bool // whether to include the console log directive (upstart 1.4)
	ExecLog         bool // use exec to force logging to somewhere reasonable
	Expect          string
	Flavor          string
	PostStartScript []string
	PreStartScript  []string
	PreStopScript   []string
	StartOn         string
	StopOn          string
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
			uo.ExecLog = true
		default:
			// Also includes "" and "ubuntu":
			uo.StartOn = "net-device-up"
			uo.StopOn = "shutdown"
			uo.ConsoleLog = true
			uo.ExecLog = false
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
		return fmt.Errorf("not able to parse Upstart template: %w", err)
	}
	return t.ExecuteTemplate(w, "UpstartConf", data)
}
