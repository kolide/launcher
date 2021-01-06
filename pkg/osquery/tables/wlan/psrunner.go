package wlan

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type PowerShell struct {
	powerShell string
}

// New create new session
func New() *PowerShell {
	ps, _ := exec.LookPath("powershell.exe")
	return &PowerShell{
		powerShell: ps,
	}
}

func runPos(ctx context.Context, output *bytes.Buffer) error {
	posh := New()
	err := posh.execute(output, simplecommand)
	if err != nil {
		fmt.Printf("error: %s\n", err)
	}

	return err
}

func (p *PowerShell) execute(out *bytes.Buffer, args ...string) error {
	args = append([]string{"-NoProfile", "-NonInteractive"}, args...)
	cmd := exec.Command(p.powerShell, args...)

	// var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("%s", stderr.String())
	}
	return err
}

const simplecommand = `
$Source = @"
public class BasicTest
{
  public static int Add(int a, int b)
    {
        return (a + b);
    }
  public int Multiply(int a, int b)
    {
    return (a * b);
    }
}
"@

Add-Type -TypeDefinition $Source
[BasicTest]::Add(4, 3)
$BasicTestObject = New-Object BasicTest
$BasicTestObject.Multiply(5, 2)
`
