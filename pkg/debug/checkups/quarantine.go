package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type quarantine struct {
	status           Status
	summary          string
	quarantineCounts map[string]int
	dirsChecked      int
}

func (q *quarantine) Name() string {
	return "Quarantine"
}

func (q *quarantine) Run(ctx context.Context, extraFh io.Writer) error {
	q.quarantineCounts = map[string]int{}

	var (
		quarantineRootDepth = map[string]int{
			`C:\Windows\System32\Drivers`: 3,
			`C:\ProgramData`:              3,

			`/Library/Application Support`: 4,
		}

		meddlesomeProcessPatterns = []string{
			`crowdstrike`,
			`opswat`,
			`defend`,
			`defense`,
			`threat`,
			`virus`,
			`quarantine`,
			`snitch`,
		}
	)

	fmt.Fprint(extraFh, "starting quarantine check\n")
	q.logMeddlesomeProccesses(ctx, extraFh, meddlesomeProcessPatterns)
	fmt.Fprintf(extraFh, "\nsearching for quarantined files:\n")

	for path, maxDepth := range quarantineRootDepth {
		fileInfo, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(extraFh, "%s does not exist\n", path)
			continue
		}

		if !fileInfo.IsDir() {
			fmt.Fprintf(extraFh, "expected %s to be a directory, but was not\n", path)
			continue
		}

		if err := q.walkDirLimited(extraFh, 0, maxDepth, path, "quarantine"); err != nil {
			q.summary = fmt.Sprintf("failed to walk %s: %s", path, err)
			q.status = Failing
			return nil
		}
	}

	fmt.Fprintf(extraFh, "%d of %d directories may contain quarantined files\n", len(q.quarantineCounts), q.dirsChecked)

	if len(q.quarantineCounts) == 0 {
		q.status = Passing
		q.summary = "no files found in quarantine"
		return nil
	}

	fmt.Fprintf(extraFh, "\nquarantined file counts:\n")
	quarantinedFiles := 0

	for path, count := range q.quarantineCounts {
		quarantinedFiles += count
		fmt.Fprintf(extraFh, "%s: %d\n", path, count)
	}

	q.status = Failing
	q.summary = fmt.Sprintf("found %d quarantined files", quarantinedFiles)
	return nil
}

func (q *quarantine) walkDirLimited(extraFh io.Writer, currentDepth, maxDepth int, dirPath, folderKeyword string) error {
	if currentDepth > maxDepth {
		return nil
	}

	q.dirsChecked++

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		// some dirs, such as /Library/Application Support/com.apple.TCC can't be read even with sudo
		// have to give terminal FDA?
		// so just move on instead of failing
		fmt.Fprintf(extraFh, "failed to read %s: %s\n", dirPath, err)
		return nil
	}

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			q.walkDirLimited(extraFh, currentDepth+1, maxDepth, filepath.Join(dirPath, dirEntry.Name()), folderKeyword)
			continue
		}

		if !strings.Contains(strings.ToLower(dirPath), folderKeyword) {
			// not in quarantine folder
			continue
		}

		// create map entry if not exists
		if _, ok := q.quarantineCounts[dirPath]; !ok {
			q.quarantineCounts[dirPath] = 1
			continue
		}

		q.quarantineCounts[dirPath]++
	}

	return nil
}

func (q *quarantine) logMeddlesomeProccesses(ctx context.Context, extraFh io.Writer, containsSubStrings []string) error {
	fmt.Fprint(extraFh, "\npossilby meddlesome proccesses:\n")
	foundMeddlesomeProcesses := false

	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting process list: %w", err)
	}

	for _, p := range ps {
		exe, _ := p.Exe()

		for _, s := range containsSubStrings {
			if !strings.Contains(strings.ToLower(exe), strings.ToLower(s)) {
				continue
			}
			foundMeddlesomeProcesses = true

			pMap := map[string]any{
				"pid":         p.Pid,
				"exe":         naIfError(p.ExeWithContext(ctx)),
				"cmdline":     naIfError(p.CmdlineSliceWithContext(ctx)),
				"create_time": naIfError(p.CreateTimeWithContext(ctx)),
				"ppid":        naIfError(p.PpidWithContext(ctx)),
				"status":      naIfError(p.StatusWithContext(ctx)),
			}

			fmt.Fprintf(extraFh, "%+v\n", pMap)
		}
	}

	if !foundMeddlesomeProcesses {
		fmt.Fprint(extraFh, "no meddlesome processes found\n")
	}

	return nil
}

func (q *quarantine) Status() Status {
	return q.status
}

func (q *quarantine) Summary() string {
	return q.summary
}

func (q *quarantine) ExtraFileName() string {
	return "quarantine.log"
}

func (q *quarantine) Data() any {
	return nil
}
