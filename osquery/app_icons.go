package osquery

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/DHowett/go-plist"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func AppIcons(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("icon"),
	}

	return table.NewPlugin("kolide_app_icons", columns, generateAppIcons(client))
}

func determineIconPath(appPath string) (string, error) {
	infoPlistPath := filepath.Join(appPath, "Contents", "Info.plist")
	infoPlistContent, err := ioutil.ReadFile(infoPlistPath)
	if err != nil {
		return "", errors.Wrap(err, "could not read Info.plist content")
	}

	var ip struct {
		IconName string `plist:"CFBundleIconFile"`
	}

	plistDecoder := plist.NewDecoder(bytes.NewReader(infoPlistContent))
	if err := plistDecoder.Decode(&ip); err != nil {
		return "", errors.Wrap(err, "could not decode plist")
	}

	if ip.IconName == "" {
		return "", errors.New("no icon set for application")
	}

	iconPath := filepath.Join(appPath, "Contents", "Resources", ip.IconName)
	if _, err := os.Stat(iconPath); err == nil {
		return iconPath, nil
	}

	iconPathExt := filepath.Join(appPath, "Contents", "Resources", fmt.Sprintf("%s.icns", ip.IconName))
	if _, err := os.Stat(iconPathExt); err == nil {
		return iconPathExt, nil
	}

	return "", errors.New("specific icon not found for application")
}

func generateAppIcons(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		var appPath string

		for column, constraints := range queryContext.Constraints {
			fmt.Fprintf(os.Stderr, fmt.Sprintf("%+v\n", constraints))
			if column == "path" {
				for _, constraint := range constraints.Constraints {
					if constraint.Operator == table.OperatorEquals {
						appPath = constraint.Expression
						break
					}
				}
			}
		}

		if appPath == "" {
			return nil, errors.New("No path constraint specified")
		}

		iconPath, err := determineIconPath(appPath)
		if err != nil {
			// was not able to find an icon for app
			return nil, errors.New("was not able to find icon for app")
		}

		iconContent, err := ioutil.ReadFile(iconPath)
		if err != nil {
			return nil, errors.Wrapf(err, "could not read file %s", iconPath)
		}

		row := map[string]string{
			"path": appPath,
			"icon": base64.StdEncoding.EncodeToString(iconContent),
		}

		return []map[string]string{row}, nil
	}
}
