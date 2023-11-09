//go:build windows
// +build windows

package allowedcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

func Commandprompt(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "cmd.exe"), arg...)
}

func Dism(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "Dism.exe"), arg...)
}

func Dsregcmd(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "dsregcmd.exe"), arg...)
}

func Echo(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	// echo on Windows is only available as a command in cmd.exe
	return newCmd(ctx, "echo", arg...), nil
}

func Ipconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "ipconfig.exe"), arg...)
}

func Powercfg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "powercfg.exe"), arg...)
}

func Powershell(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "WindowsPowerShell", "v1.0", "powershell.exe"), arg...)
}

func Repcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("PROGRAMFILES"), "Confer", "repcli"), arg...)
}

func Secedit(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "SecEdit.exe"), arg...)
}

func Taskkill(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEM32"), "taskkill.exe"), arg...)
}

func Zerotiercli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	// For windows, "-q" should be prepended before all other args
	return validatedCommand(ctx, filepath.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "ZeroTier", "One", "zerotier-one_x64.exe"), append([]string{"-q"}, arg...)...)
}
