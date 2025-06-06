package packagekit

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"
)

//go:embed assets/init.sh
var initdTemplate []byte

func RenderInit(ctx context.Context, w io.Writer, initOptions *InitOptions) error {
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
		return fmt.Errorf("not able to parse initd template: %w", err)
	}
	return t.ExecuteTemplate(w, "initd", data)

}
