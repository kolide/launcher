//go:build windows
// +build windows

package notify

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"text/template"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent"
)

type windowsNotifier struct {
	iconFilepath string
	logger       log.Logger
	interrupt    chan struct{}
}

func newOsSpecificNotifier(logger log.Logger, iconFilepath string) *windowsNotifier {
	return &windowsNotifier{
		iconFilepath: iconFilepath,
		logger:       logger,
		interrupt:    make(chan struct{}),
	}
}

func (w *windowsNotifier) Listen() error {
	for {
		select {
		case <-w.interrupt:
			return nil
		}
	}
}

func (w *windowsNotifier) Interrupt(err error) {
	w.interrupt <- struct{}{}
}

func (w *windowsNotifier) SendNotification(title, body, actionUri string) error {
	notification := struct {
		Title               string
		Body                string
		Icon                string
		ActivationArguments string
	}{
		Title: title,
		Body:  body,
	}

	if w.iconFilepath != "" {
		notification.Icon = w.iconFilepath
	}

	if actionUri != "" {
		notification.ActivationArguments = actionUri
	}

	// The following comes from go-toast, which we can't use on its own anymore due to its
	// use of os.TempDir() to invoke the temporary script. This is a stripped-down implementation
	// that uses a temporary directory that launcher will have permission to access.
	notifyTmpl := template.New("notify")
	notifyTmpl.Parse(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$APP_ID = 'Kolide'
$template = @"
<toast activationType="protocol" launch="{{.ActivationArguments}}" duration="short">
    <visual>
        <binding template="ToastGeneric">
            {{if .Icon}}
            <image placement="appLogoOverride" src="{{.Icon}}" />
            {{end}}
            {{if .Title}}
            <text><![CDATA[{{.Title}}]]></text>
            {{end}}
            {{if .Body}}
            <text><![CDATA[{{.Body}}]]></text>
            {{end}}
        </binding>
    </visual>
	<audio silent="true" />
</toast>
"@
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)
    `)
	var scriptBytes bytes.Buffer
	err := notifyTmpl.Execute(&scriptBytes, notification)
	if err != nil {
		return fmt.Errorf("could not execute xml template: %w", err)
	}

	tmpDir, err := agent.MkdirTemp("")
	if err != nil {
		return fmt.Errorf("could not make temporary directory for powershell script: %w", err)
	}

	tmpScriptFile := filepath.Join(tmpDir, fmt.Sprintf("%s.ps1", ulid.New()))
	bomUtf8 := []byte{0xEF, 0xBB, 0xBF}
	out := append(bomUtf8, scriptBytes.Bytes()...)
	if err := os.WriteFile(tmpScriptFile, out, 0600); err != nil {
		return fmt.Errorf("could not write temporary powershell script to %s: %w", tmpScriptFile, err)
	}

	// TODO: fix PATH so we don't have to hardcode powershell's location
	cmd := exec.Command("C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe", "-ExecutionPolicy", "Bypass", "-File", tmpScriptFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not run powershell to create notification: %w", err)
	}

	return nil
}
