//go:build windows

package allowedcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

var CommandPrompt = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe"))

var Dism = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "Dism.exe"))

var Dsregcmd = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "dsregcmd.exe"))

// echoCommand implements AllowedCommand for Windows where echo is a shell builtin. It skips
// The path searching behavior.
type echoCommand struct {
	env []string
}

func (echoCommand) Name() string { return "echo" }

func (ac echoCommand) Cmd(ctx context.Context, arg ...string) (*TracedCmd, error) {
	return newCmd(ctx, ac.env, "echo", arg...), nil
}

func (ac echoCommand) WithEnv(env string) echoCommand {
	ac.env = append(ac.env, env)
	return ac
}

var Echo AllowedCommand = echoCommand{}

var Ipconfig = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "ipconfig.exe"))

var Powercfg = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "powercfg.exe"))

var Powershell = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))

var Repcli = newAllowedCommand(filepath.Join(os.Getenv("PROGRAMFILES"), "Confer", "repcli"))

var Secedit = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "SecEdit.exe"))

var Taskkill = newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "taskkill.exe"))

var Zscli = newAllowedCommand(filepath.Join(os.Getenv("PROGRAMFILES"), "Zscaler", "ZSACli", "ZSACli.exe"))

// type zerotierCli implements AllowedCommand. It uses a custom function because
// on windows, "-q" should be prepended before all other args
type zerotierCli struct{}

func (zerotierCli) Name() string { return "ZerotierCli" }
func (ac zerotierCli) Cmd(ctx context.Context, arg ...string) (*TracedCmd, error) {
	knownPaths := []string{
		filepath.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "ZeroTier", "One", "zerotier-one_x64.exe"),
	}

	cmdpath, err := findExecutable(knownPaths)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ac.Name(), err)
	}

	// For windows, "-q" should be prepended before all other args
	return newCmd(ctx, nil, cmdpath, append([]string{"-q"}, arg...)...), nil
}

var ZerotierCli = zerotierCli{}
