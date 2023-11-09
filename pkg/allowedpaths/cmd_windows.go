//go:build windows
// +build windows

package allowedpaths

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

func Commandprompt(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\cmd.exe`)
	if err != nil {
		return nil, fmt.Errorf("cmd.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dism(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\Dism.exe`)
	if err != nil {
		return nil, fmt.Errorf("dism.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Dsregcmd(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\dsregcmd.exe`)
	if err != nil {
		return nil, fmt.Errorf("dsregcmd.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Echo(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	// echo on Windows is only available as a command in cmd.exe
	return newCmd(ctx, "echo", arg...), nil
}

func Ipconfig(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\ipconfig.exe`)
	if err != nil {
		return nil, fmt.Errorf("ipconfig.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Powercfg(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\powercfg.exe`)
	if err != nil {
		return nil, fmt.Errorf("powercfg.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Powershell(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`)
	if err != nil {
		return nil, fmt.Errorf("powershell.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Repcli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(filepath.Join("Program Files", "Confer", "repcli"))
	if err != nil {
		return nil, fmt.Errorf("repcli not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Secedit(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\SecEdit.exe`)
	if err != nil {
		return nil, fmt.Errorf("secedit.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Taskkill(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(`C:\Windows\System32\taskkill.exe`)
	if err != nil {
		return nil, fmt.Errorf("taskkill.exe not found: %w", err)
	}
	return newCmd(ctx, fullPathToCmdValidated, arg...), nil
}

func Zerotiercli(ctx context.Context, arg ...string) (*exec.Cmd, error) {
	fullPathToCmdValidated, err := validatedPath(path.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "ZeroTier", "One", "zerotier-one_x64.exe"))
	if err != nil {
		return nil, fmt.Errorf("zerotier-cli not found: %w", err)
	}

	// For windows, "-q" should be prepended before all other args
	return newCmd(ctx, fullPathToCmdValidated, []string{"-q"}, arg...), nil
}
