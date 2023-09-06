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
	q.quarantineCounts = make(map[string]int)

	var (
		quarantinePathDepths = map[string]int{
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

	for path, maxDepth := range quarantinePathDepths {
		fileInfo, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(extraFh, "%s does not exist\n", path)
			continue
		}

		if !fileInfo.IsDir() {
			fmt.Fprintf(extraFh, "expected %s to be a directory, but was not\n", path)
			continue
		}

		q.checkDirs(extraFh, 0, maxDepth, path, "quarantine")
	}

	fmt.Fprintf(extraFh, "total directories checked: %d\n", q.dirsChecked)

	if len(q.quarantineCounts) == 0 {
		q.status = Passing
		q.summary = "no quarantine directories found"
		fmt.Fprint(extraFh, "no quarantine directories found\n")
		return nil
	}

	fmt.Fprintf(extraFh, "quarantine directory paths and file counts:\n")

	totalQuarantinedFiles := 0

	for path, count := range q.quarantineCounts {
		fmt.Fprintf(extraFh, "%s: %d\n", path, count)
		totalQuarantinedFiles += count
	}

	if totalQuarantinedFiles == 0 {
		q.status = Passing
		q.summary = "no files found in quarantine directories"
		return nil
	}

	q.status = Failing
	q.summary = fmt.Sprintf("found %d quarantined files", totalQuarantinedFiles)
	return nil
}

// Recursively scans dir to given max depth. Creates entry for each dir whose path contains the directoryKeyword.
// Increments quarantine.quarantineCounts for each file found in folder and descendant folders.
func (q *quarantine) checkDirs(extraFh io.Writer, currentDepth, maxDepth int, dirPath, directoryKeyword string) {
	if currentDepth > maxDepth {
		return
	}

	q.dirsChecked++

	dirNameContainsKeyword := strings.Contains(strings.ToLower(dirPath), directoryKeyword)

	// add entry for each dir that contains the keyword
	if dirNameContainsKeyword {
		// create map entry if not exists
		if _, ok := q.quarantineCounts[dirPath]; !ok {
			q.quarantineCounts[dirPath] = 0
		}
	}

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		// some dirs, such as /Library/Application Support/com.apple.TCC can't be read even with sudo
		// have to give terminal FDA?
		// so just move on instead of failing
		fmt.Fprintf(extraFh, "failed to read %s: %s\n", dirPath, err)
		return
	}

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			q.checkDirs(extraFh, currentDepth+1, maxDepth, filepath.Join(dirPath, dirEntry.Name()), directoryKeyword)
			continue
		}

		if !dirNameContainsKeyword {
			// not in quarantine dir
			continue
		}

		q.quarantineCounts[dirPath]++
	}
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