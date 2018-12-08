package packagekit

import (
	"context"
	"io"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func RenderInit(ctx context.Context, w io.Writer, initOptions *InitOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.RenderInit")
	defer span.End()

	initdTemplate := `#!/bin/sh
set -e
NAME="{{.Common.Identifier}}"
DAEMON="{{.Common.Path}}"
DAEMON_OPTS="{{ StringsJoin .Common.Flags " \\\n" }}"

{{- range $key, $value := .Common.Environment }}
{{$key}}={{$value}}
export {{$key}}
{{- end }}

PATH="${PATH:+$PATH:}/usr/sbin:/sbin"
export PATH

is_running() {
    start-stop-daemon --status --exec $DAEMON
}
case "$1" in
  start)
        echo "Starting daemon: "$NAME
        start-stop-daemon --start --quiet --background --exec $DAEMON -- $DAEMON_OPTS
        ;;
  stop)
        echo "Stopping daemon: "$NAME
        start-stop-daemon --stop --quiet --oknodo --exec $DAEMON
        ;;
  restart)
        echo "Restarting daemon: "$NAME
        start-stop-daemon --stop --quiet --oknodo --retry 30 --exec $DAEMON
        start-stop-daemon --start --quiet --background --exec $DAEMON -- $DAEMON_OPTS
        ;;
  status)
    if is_running; then
        echo "Running"
    else
        echo "Stopped"
        exit 1
    fi
    ;;
  *)
        echo "Usage: "$1" {start|stop|restart|status}"
        exit 1
esac

exit 0
`

	var data = struct {
		Common InitOptions
	}{
		Common: *initOptions,
	}

	funcsMap := template.FuncMap{
		"StringsJoin": strings.Join,
	}

	t, err := template.New("initd").Funcs(funcsMap).Parse(initdTemplate)
	if err != nil {
		return errors.Wrap(err, "not able to parse initd template")
	}
	return t.ExecuteTemplate(w, "initd", data)

}
