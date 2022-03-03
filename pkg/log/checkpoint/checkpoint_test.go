package checkpoint

import (
	"testing"

	"github.com/stretchr/testify/mock"
)

type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) Log(keyvals ...interface{}) error {
	m.Called(keyvals...)
	return nil
}

func TestLogCheckPoint(t *testing.T) {
	t.Parallel()

	mockLogger := new(mockLogger)

	mockLogger.On("Log", "msg", "log checkpoint started",
		"hostname", hostName(),
		"notableFiles", fileNamesInDirs(notableFileDirs...)).Return(nil)

	logCheckPoint(mockLogger)

	mockLogger.AssertCalled(t, "Log",
		"msg", "log checkpoint started",
		"hostname", hostName(),
		"notableFiles", fileNamesInDirs(notableFileDirs...))
}
