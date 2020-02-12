package packagekit

import (
	"context"
	"io"
	"strings"
	"text/template"

	"github.com/kolide/launcher/pkg/packagekit/internal"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func RenderInit(ctx context.Context, w io.Writer, initOptions *InitOptions) error {
	_, span := trace.StartSpan(ctx, "packagekit.RenderInit")
	defer span.End()

	initdTemplate, err := internal.Asset("internal/assets/init.sh")
	if err != nil {
		return errors.Wrapf(err, "Failed to get template named internal/assets/init.sh")
	}

	var data = struct {
		Common InitOptions
	}{
		Common: *initOptions,
	}

	funcsMap := template.FuncMap{
		"StringsJoin": strings.Join,
	}

	t, err := template.New("initd").Funcs(funcsMap).Parse(string(initdTemplate))
	if err != nil {
		return errors.Wrap(err, "not able to parse initd template")
	}
	return t.ExecuteTemplate(w, "initd", data)

}
