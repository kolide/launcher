//go:build windows
// +build windows

package notify

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/go-kit/kit/log/level"
)

//go:embed assets/toast.ps1
var toastTemplate []byte

type toastNotification struct {
	Title string
	Body  string
}

func (n *Notifier) sendNotification(title, body string) {
	t, err := template.New("toast").Parse(string(toastTemplate))
	if err != nil {
		level.Error(n.logger).Log("msg", "could not parse toast template to send notification", "title", title, "err", err)
		return
	}

	dir, err := os.MkdirTemp("", "toast")
	if err != nil {
		level.Error(n.logger).Log("msg", "could not create temp dir to send toast", "title", title, "err", err)
		return
	}
	defer os.RemoveAll(dir)

	toastScript := filepath.Join(dir, "toast.ps1")
	fh, err := os.Create(toastScript)
	if err != nil {
		level.Error(n.logger).Log("msg", "could not create toast script file", "title", title, "err", err)
		return
	}
	defer fh.Close()

	err = t.ExecuteTemplate(fh, "toast", toastNotification{Title: title, Body: body})
	if err != nil {
		level.Error(n.logger).Log("msg", "could not execute toast template", "title", title, "err", err)
		return
	}

	pwsh, err := exec.LookPath("powershell.exe")
	if err != nil {
		level.Error(n.logger).Log("msg", "could not find powershell to send notification", "title", title, "err", err)
		return
	}

	cmd := exec.Command(pwsh, "-ExecutionPolicy", "Bypass", "-File", toastScript)
	if err := cmd.Run(); err != nil {
		level.Error(n.logger).Log("msg", "could not send notification via powershell", "title", title, "err", err)
	}
}
