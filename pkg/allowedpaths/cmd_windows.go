//go:build windows
// +build windows

package allowedpaths

var knownPaths = map[string]map[string]bool{
	"cmd.exe": {
		`C:\Windows\System32\cmd.exe`: true,
	},
	"dism.exe": {
		`C:\Windows\System32\Dism.exe`: true,
	},
	"ipconfig.exe": {
		`C:\Windows\System32\ipconfig.exe`: true,
	},
	"powercfg.exe": {
		`C:\Windows\System32\powercfg.exe`: true,
	},
	"powershell.exe": {
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`: true,
	},
	"secedit.exe": {
		`C:\Windows\System32\SecEdit.exe`: true,
	},
	"taskkill.exe": {
		`C:\Windows\System32\taskkill.exe`: true,
	},
}

var knownPathPrefixes = []string{
	`C:\Windows\System32`,
}
