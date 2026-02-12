//go:build windows

package allowedcmd

import (
	"context"
	"os"
	"path/filepath"
)

var CommandPrompt = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe"))

var Dism = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "Dism.exe"))

var Dsregcmd = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "dsregcmd.exe"))

// echoCommand implements AllowedCommand for Windows where echo is a shell builtin.
type echoCommand struct{}

func (echoCommand) Name() string { return "echo" }

func (echoCommand) Cmd(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return newCmd(ctx, nil, "echo", arg...), nil
}

var Echo AllowedCommand = echoCommand{}

var Ipconfig = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "ipconfig.exe"))

var Powercfg = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "powercfg.exe"))

var Powershell = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))

var Repcli = newAllowedCommand(filepath.Join(os.Getenv("PROGRAMFILES"), "Confer", "repcli"))

var Secedit = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "SecEdit.exe"))

var Taskkill = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "taskkill.exe"))

var Zscli = newAllowedCommand(filepath.Join(os.Getenv("PROGRAMFILES"), "Zscaler", "ZSACli", "ZSACli.exe"))

var zerotierCliPath = newAllowedCommand(filepath.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "ZeroTier", "One", "zerotier-one_x64.exe"))

func ZerotierCli(ctx context.Context, arg ...string) (*TracedCmd, error) {
	// For windows, "-q" should be prepended before all other args
	return zerotierCliPath.Cmd(ctx, append([]string{"-q"}, arg...)...)
}
