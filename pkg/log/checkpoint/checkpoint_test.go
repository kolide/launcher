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
	mockLogger := new(mockLogger)

	// tried to figure out a way to put this in a variable ... but couldn't figure out syntax, closest I got was:
	// args := []interface{}{"msg", "log checkpoint started", "hostname", hostName(), "notableFiles", notableFiles()}
	// however it didn't match what go called by logCheckPoint()

	mockLogger.On("Log", "msg", "log checkpoint started",
		"hostname", hostName(),
		"notableFiles", notableFilePaths()).Return(nil)

	logCheckPoint(mockLogger)

	mockLogger.AssertCalled(t, "Log",
		"msg", "log checkpoint started",
		"hostname", hostName(),
		"notableFiles", notableFilePaths())
}
