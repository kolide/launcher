package checkups

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/ee/agent/types"
)

type desktopMenu struct {
	k       types.Knapsack
	status  Status
	summary string
}

func (d *desktopMenu) Name() string {
	return "Desktop Menu"
}

func (d *desktopMenu) Run(_ context.Context, fullFH io.Writer) error {
	menuJsonPath := filepath.Join(d.k.RootDirectory(), "menu.json")

	d.status = Failing

	if _, err := os.Stat(menuJsonPath); err != nil {
		d.summary = fmt.Sprintf("failed to stat menu.json: %s", err)
		return nil
	}

	file, err := os.Open(menuJsonPath)
	if err != nil {
		d.summary = fmt.Sprintf("failed to open %s: %s", menuJsonPath, err)
		return nil
	}

	menuJson, err := io.ReadAll(file)
	if err != nil {
		d.summary = fmt.Sprintf("failed to read menu.json: %s", err)
		return nil
	}

	if _, err := io.Copy(fullFH, bytes.NewBuffer(menuJson)); err != nil {
		d.summary = fmt.Sprintf("failed to copy menu.json to file handler: %s", err)
		return nil
	}

	// just unmarshall the contents to make sure it's valid json
	var unmarshalledMenuJson any
	if err := json.Unmarshal(menuJson, &unmarshalledMenuJson); err != nil {
		d.summary = fmt.Sprintf("failed to unmarshal menu.json: %s", err)
		return nil
	}

	d.status = Passing
	d.summary = "menu.json exists and is valid json"

	return nil
}

func (d *desktopMenu) Status() Status {
	return d.status
}

func (d *desktopMenu) Summary() string {
	return d.summary
}

func (d *desktopMenu) ExtraFileName() string {
	return "menu.json"
}

func (d *desktopMenu) Data() any {
	return nil
}
