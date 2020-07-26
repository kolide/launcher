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
	}
}

func (uo *upstartOptions) TemplateName() string {
	switch uo.Flavor {
	case "amazon-ami":
		return "internal/assets/upstart-amazon.sh"
	default:
		return "internal/assets/upstart.sh"
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

	templateName := uOptions.TemplateName()
	upstartTemplate, err := internal.Asset(templateName)
	if err != nil {
		return errors.Wrapf(err, "Failed to get template named %s", templateName)
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
