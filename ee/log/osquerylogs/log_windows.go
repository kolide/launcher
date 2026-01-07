//go:build windows

package osquerylogs

func (l *OsqueryLogAdapter) runAndLogPs(_ string) {}

func (l *OsqueryLogAdapter) runAndLogLsofByPID(_ string) {}

func (l *OsqueryLogAdapter) runAndLogLsofOnPidfile() {}
