package checkpoint

import (
	"fmt"
	"io/ioutil"
	"runtime"
)

// presence of files in these directories can cause issues with the launcher
// can update this during test cases to check other dirs
var notableFileDirs = map[string][]string{
	"darwin": {"/var/osquery", "/etc/osquery"},
	"linux":  {"/var/osquery", "/etc/osquery"},
	// TODO: add windows dirs
}

func notableFilePaths() []string {
	if val, ok := notableFileDirs[runtime.GOOS]; ok {
		return fileNamesInDirs(val...)
	}
	return []string{notableFilesDirNotDefinedMsg(runtime.GOOS)}
}

func notableFilesDirNotDefinedMsg(os string) string {
	const msgFmt = "notable file dirs not defined for %v"
	return fmt.Sprintf(msgFmt, os)
}

func fileNamesInDirs(dirnames ...string) []string {
	results := []string{}

	for _, dirname := range dirnames {
		files, err := ioutil.ReadDir(dirname)

		if err != nil {
			results = append(results, err.Error())
			continue
		}

		for _, file := range files {
			// hmm ... do we want to make this recursive and walk the tree?
			results = append(results, dirname+"/"+file.Name())
		}
	}

	return results
}
