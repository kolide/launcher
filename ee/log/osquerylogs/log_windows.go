//go:build windows

package osquerylogs

func (l *OsqueryLogAdapter) runAndLogPs(_ string) {
	return
}

func (l *OsqueryLogAdapter) runAndLogLsofByPID(_ string) {
	return
}

func (l *OsqueryLogAdapter) runAndLogLsofOnPidfile() {
	return
}
