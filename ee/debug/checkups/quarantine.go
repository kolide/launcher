package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// quarantine:
// Recursively scans common installation directories to to a given depth.
// Reports and directories that have the word "quarantine" in their path and the number of files and their names they contain.
// Warns if any files are found in the above directories.
// Reports possible "meddlesome" processes for information purposes (does not fail due to proccesses running)

// It's difficult to keep track of every possible Anti-Virus or EDRs quarantine directory, but they all seem
// to have "quarantine" in their name. So we just look for that some where in the dir path. The suspicion
// is that some programs will quarantine osquery. Unfortunalty, we typically can't see the names of the files
// that were quarantined. So if we do find quarantined files, we'll fail and would ask the user to check and
// see if osquery was quarantined.

type quarantine struct {
	status                     Status
	summary                    string
	quarantineDirPathFilenames map[string][]string
	dirsChecked                int
}

func (q *quarantine) Name() string {
	return "Quarantine"
}

func (q *quarantine) searchPathDepths() map[string]int {
	switch runtime.GOOS {
	case "windows":
		return map[string]int{
			// Crowdstrike: C:\Windows\System32\Drivers\CrowdStrike\Quarantine
			`C:\Windows\System32\Drivers`: 3,
			// Malwarebytes: C:\ProgramData\Malwarebytes\MBAMService\Quarantine
			// Windows Defender: C:\ProgramData\Microsoft\Windows Defender\Quarantine
			`C:\ProgramData`: 3,
		}
	case "darwin":
		return map[string]int{
			// Crowdstrike: /Library/Application Support/CrowdStrike/Falcon/Quarantine
			`/Library/Application Support`: 4,
		}
	case "linux":
		return map[string]int{
			// Malwarebytes: /var/lib/mblinux/quarantine
			`/var/lib`: 3,
		}
	default:
		return make(map[string]int)
	}
}

func (q *quarantine) Run(ctx context.Context, extraFh io.Writer) error {
	q.quarantineDirPathFilenames = make(map[string][]string)

	var (
		meddlesomeProcessPatterns = []string{
			`crowdstrike`,
			`opswat`,
			`defend`,
			`defense`,
			`threat`,
			`virus`,
			`quarantine`,
			`snitch`,
			`action1`,
			`nessus`,
			`dnsfilter`,
			// carbon black possible processes
			`cbagent`,
			`carbonblack`,
			`repmgr`,
			`repwsc`,
			`cb.exe`,
			`cbdaemon`,
			`cbOsxSensorService`,
		}
	)

	fmt.Fprint(extraFh, "starting quarantine check\n")
	q.logMeddlesomeProccesses(ctx, extraFh, meddlesomeProcessPatterns)
	fmt.Fprintf(extraFh, "\nsearching for quarantined files:\n")

	for path, maxDepth := range q.searchPathDepths() {
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

	if len(q.quarantineDirPathFilenames) == 0 {
		q.status = Passing
		q.summary = "no quarantine directories found"
		fmt.Fprint(extraFh, "no quarantine directories found\n")
		return nil
	}

	fmt.Fprintf(extraFh, "quarantine directory paths and files:\n")

	totalQuarantinedFiles := 0

	for path, fileNames := range q.quarantineDirPathFilenames {
		fmt.Fprintf(extraFh, "%s: %d files\n", path, len(fileNames))
		totalQuarantinedFiles += len(fileNames)

		for _, fileName := range fileNames {
			fmt.Fprintf(extraFh, "  %s\n", fileName)
		}
	}

	if totalQuarantinedFiles == 0 {
		q.status = Passing
		q.summary = "no files found in quarantine directories"
		return nil
	}

	q.status = Warning
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
		if _, ok := q.quarantineDirPathFilenames[dirPath]; !ok {
			q.quarantineDirPathFilenames[dirPath] = make([]string, 0)
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

		// typically AVs will rename the file to a guid and store meta data some where
		// so just log the file count
		q.quarantineDirPathFilenames[dirPath] = append(q.quarantineDirPathFilenames[dirPath], dirEntry.Name())
	}
}

func (q *quarantine) logMeddlesomeProccesses(ctx context.Context, extraFh io.Writer, containsSubStrings []string) error {
	fmt.Fprint(extraFh, "\npossibly meddlesome processes:\n")
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
