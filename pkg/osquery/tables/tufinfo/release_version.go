package tufinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/osquery/osquery-go/plugin/table"
	"github.com/theupdateframework/go-tuf/data"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
)

const tufReleaseVersionTableName = "kolide_tuf_release_version"

func TufReleaseVersionTable(flags types.Flags) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("binary"),
		table.TextColumn("operating_system"),
		table.TextColumn("architecture"),
		table.TextColumn("channel"),
		table.TextColumn("target"),
	}

	return table.NewPlugin(tufReleaseVersionTableName, columns, generateTufReleaseVersionTable(flags))
}

func generateTufReleaseVersionTable(flags types.Flags) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		results := []map[string]string{}

		for _, binary := range []string{"launcher", "osqueryd"} {
			tufTargetsFile := filepath.Join(tuf.LocalTufDirectory(flags.RootDirectory()), "targets.json")

			targetFileBytes, err := os.ReadFile(tufTargetsFile)
			if err != nil {
				return nil, fmt.Errorf("cannot read file %s: %w", tufTargetsFile, err)
			}

			var signedTargetFile data.Signed
			if err := json.Unmarshal(targetFileBytes, &signedTargetFile); err != nil {
				return nil, fmt.Errorf("cannot unmarshal target file %s: %w", tufTargetsFile, err)
			}

			var targets data.Targets
			if err := json.Unmarshal(signedTargetFile.Signed, &targets); err != nil {
				return nil, fmt.Errorf("cannot unmarshal signed targets from target file %s: %w", tufTargetsFile, err)
			}

			targetsToCheck := expectedReleaseTargets(binary)
			for targetFileName, targetFileMetadata := range targets.Targets {
				if _, ok := targetsToCheck[targetFileName]; !ok {
					continue
				}

				parts := strings.Split(targetFileName, "/")
				if len(parts) != 5 {
					// Shouldn't happen given the check above, but just in case
					continue
				}

				var metadata tuf.ReleaseFileCustomMetadata
				if err := json.Unmarshal(*targetFileMetadata.Custom, &metadata); err != nil {
					return nil, fmt.Errorf("cannot extract target metadata from file %s: %w", targetFileName, err)
				}

				results = append(results, map[string]string{
					"binary":           binary,
					"operating_system": parts[1],
					"architecture":     parts[2],
					"channel":          parts[3],
					"target":           metadata.Target,
				})
			}
		}

		return results, nil
	}
}

func expectedReleaseTargets(binary string) map[string]bool {
	targets := make(map[string]bool, 0)
	for _, operatingSystem := range []string{"darwin", "windows", "linux"} {
		for _, arch := range []string{"universal", "arm64", "amd64"} {
			if operatingSystem != "darwin" && arch == "universal" {
				continue
			}
			for _, channel := range []string{"stable", "beta", "alpha", "nightly"} {
				targets[path.Join(binary, operatingSystem, arch, channel, "release.json")] = true
			}
		}

	}

	return targets
}
