//go:build windows
// +build windows

package log

func (l *OsqueryLogAdapter) runAndLogPs(_ string) {
	return
}

func (l *OsqueryLogAdapter) runAndLogLsofByPID(_ string) {
	return
}

func (l *OsqueryLogAdapter) runAndLogLsofOnPidfile() {
	return
}
