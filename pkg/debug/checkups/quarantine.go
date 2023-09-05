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
	status  Status
	summary string
}

func (q *quarantine) Name() string {
	return "Quarantine"
}

func (q *quarantine) Run(ctx context.Context, extraFh io.Writer) error {

	var (
		rootsToScanForQuarantineDirs = []string{
			`C:\Windows\System32\Drivers`,
			`C:\ProgramData`,

			`/Library/Application Support`,
		}

		fileNamesToMatch = []string{
			`osquery`,
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

	var quarantineDirs []string

	// find all the folders that contain the word quarantine
	for _, root := range rootsToScanForQuarantineDirs {
		fileInfo, err := os.Stat(root)
		if err != nil {
			fmt.Fprintf(extraFh, "%s does not exist\n", root)
			continue
		}

		if !fileInfo.IsDir() {
			fmt.Fprintf(extraFh, "expected %s to be a directory, but was not\n", root)
			continue
		}

		dirs := q.findDirsThatContain(extraFh, root, "quarantine")
		quarantineDirs = append(quarantineDirs, dirs...)
	}

	if len(quarantineDirs) == 0 {
		q.status = Passing
		status := "No quarantine directories found"
		fmt.Fprint(extraFh, status)
		q.summary = status
		return nil
	}

	// scan quarantine dirs for files that might relate to us
	for _, dir := range quarantineDirs {
		if pass, result := q.checkDirForQuarantinedFiles(extraFh, dir, fileNamesToMatch); !pass {
			q.status = Failing
			q.summary = result
			return nil
		}
	}

	q.status = Passing
	q.summary = "No notable files found in quarantine directories"
	return nil
}

func (q *quarantine) checkDirForQuarantinedFiles(extraFh io.Writer, path string, quarantineFileNamesToSearchFor []string) (bool, string) {
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		// if we can't read the directory, we can't do much else, error
		result := fmt.Sprintf("failed to read directory %s: %s", path, err)
		fmt.Fprint(extraFh, result)
		return false, result
	}

	fmt.Fprintf(extraFh, "found %d files in %s\n", len(dirEntries), path)

	if len(dirEntries) == 0 {
		return true, ""
	}

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			if pass, result := q.checkDirForQuarantinedFiles(extraFh, filepath.Join(path, dirEntry.Name()), quarantineFileNamesToSearchFor); !pass {
				return pass, result
			}

			continue
		}

		for _, pattern := range quarantineFileNamesToSearchFor {
			fileName := strings.ToLower(dirEntry.Name())
			pattern := strings.ToLower(pattern)

			if strings.Contains(fileName, pattern) {
				result := fmt.Sprintf("found file %s in folder %s that contained the string %s", dirEntry.Name(), path, pattern)
				fmt.Fprint(extraFh, result)
				return false, result
			}
		}

		fmt.Fprintf(extraFh, "no notable files found in %s", dirEntry.Name())
	}

	return true, ""
}

func (q *quarantine) findDirsThatContain(extraFh io.Writer, rootDir, substr string) []string {
	var matchingPaths []string
	dirCount := 0

	if err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		dirCount++

		if err != nil {
			fmt.Fprintf(extraFh, "error walking directory: %v\n", err)
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		if strings.Contains(strings.ToLower(d.Name()), strings.ToLower(substr)) {
			matchingPaths = append(matchingPaths, path)
		}

		return nil
	}); err != nil {
		return matchingPaths
	}

	fmt.Fprintf(extraFh, "%d out of %d scanned directories starting at root %s contained %s\n", len(matchingPaths), dirCount, rootDir, substr)
	return matchingPaths
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
