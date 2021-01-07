package wlan

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/pkg/errors"
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
	dir, err := ioutil.TempDir("", "nativewifi")
	if err != nil {
		return errors.Wrap(err, "creating nativewifi tmp dir")
	}
	defer os.RemoveAll(dir)

	outputFile := filepath.Join(dir, "nativewificode.cs")
	nativeCodeFile, err := os.Create(outputFile)
	if err != nil {
		return errors.Wrap(err, "creating file for native wifi code")
	}
	defer os.Remove(nativeCodeFile.Name())

	_, err = nativeCodeFile.WriteString(nativeWiFiCode)
	if err != nil {
		return errors.Wrap(err, "writing native code file")
	}

	tmpl, err := template.New("command").Parse(getBSSIDCommandTemplate)
	if err != nil {
		return errors.Wrap(err, "parsing template")
	}
	commandOpts := struct {
		NativeCodePath string
	}{NativeCodePath: nativeCodeFile.Name()}
	var command bytes.Buffer
	if err := tmpl.ExecuteTemplate(&command, "command", commandOpts); err != nil {
		return errors.Wrap(err, "executing template")
	}

	err = posh.execute(output, command.String())
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
