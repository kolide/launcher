package log

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogRawLogRecord(t *testing.T) {
	t.Parallel()

	logTimeString := "2025-07-24T19:34:15.741622Z"

	// Need to explicitly make it a float64 because of JSON number processing
	fileLineInt := 238
	var fileLineAsFloat float64 = 238

	for _, tt := range []struct {
		testCaseName          string
		rawLogRecord          []byte
		expectedLogMessage    string
		expectedLogLevel      slog.Level
		expectedLogAttributes []slog.Attr
	}{
		{
			testCaseName:       "valid debug-level log",
			rawLogRecord:       fmt.Appendf(nil, `{"time":"%s","level":"DEBUG","source":{"function":"github.com/kolide/launcher/ee/observability/exporter.(*TelemetryExporter).addAttributesFromEnrollmentDetails","file":"/Users/runner/work/launcher/launcher/ee/observability/exporter/exporter.go","line":%d},"msg":"added attributes from enrollment details"}`, logTimeString, fileLineInt),
			expectedLogMessage: "added attributes from enrollment details",
			expectedLogLevel:   slog.LevelDebug,
			expectedLogAttributes: []slog.Attr{
				slog.String("original.time", logTimeString),
				slog.Any("original.source", map[string]any{
					"file":     "/Users/runner/work/launcher/launcher/ee/observability/exporter/exporter.go",
					"function": "github.com/kolide/launcher/ee/observability/exporter.(*TelemetryExporter).addAttributesFromEnrollmentDetails",
					"line":     fileLineAsFloat,
				}),
			},
		},
		{
			testCaseName:       "valid info-level log",
			rawLogRecord:       fmt.Appendf(nil, `{"time":"%s","level":"INFO","source":{"function":"github.com/kolide/launcher/ee/log/osquerylogs.(*OsqueryLogAdapter).Write","file":"/Users/runner/work/launcher/launcher/ee/log/osquerylogs/log.go","line":%d},"msg":"I0725 09:58:21.866813 1883975680 scheduler.cpp:201] Found results for query: pack:kolide_log_pipeline:example_noisy_pack:current_time","component":"osquery","osqlevel":"stderr","registration_id":"default","instance_run_id":"01K10511D8DCCGW37X8SR5WKC3","caller":"scheduler.cpp:201"}`, logTimeString, fileLineInt),
			expectedLogMessage: "I0725 09:58:21.866813 1883975680 scheduler.cpp:201] Found results for query: pack:kolide_log_pipeline:example_noisy_pack:current_time",
			expectedLogLevel:   slog.LevelInfo,
			expectedLogAttributes: []slog.Attr{
				slog.String("original.time", logTimeString),
				slog.Any("original.source", map[string]any{
					"file":     "/Users/runner/work/launcher/launcher/ee/log/osquerylogs/log.go",
					"function": "github.com/kolide/launcher/ee/log/osquerylogs.(*OsqueryLogAdapter).Write",
					"line":     fileLineAsFloat,
				}),
				slog.String("original.component", "osquery"),
				slog.String("original.osqlevel", "stderr"),
				slog.String("original.registration_id", "default"),
				slog.String("original.instance_run_id", "01K10511D8DCCGW37X8SR5WKC3"),
				slog.String("original.caller", "scheduler.cpp:201"),
			},
		},
		{
			testCaseName:       "valid warn-level log",
			rawLogRecord:       fmt.Appendf(nil, `{"time":"%s","level":"WARN","source":{"function":"github.com/kolide/launcher/pkg/osquery/runtime.(*OsqueryInstance).Launch.func4","file":"/Users/runner/work/launcher/launcher/pkg/osquery/runtime/osqueryinstance.go","line":%d},"msg":"error running osquery command","component":"osquery_instance","registration_id":"default","instance_run_id":"01K0YYVJK0G5A33RSC8VCKMK26","path":"/usr/local/kolide-nababe-k2/bin/osqueryd","err":"signal: killed"}`, logTimeString, fileLineInt),
			expectedLogMessage: "error running osquery command",
			expectedLogLevel:   slog.LevelWarn,
			expectedLogAttributes: []slog.Attr{
				slog.String("original.time", logTimeString),
				slog.Any("original.source", map[string]any{
					"file":     "/Users/runner/work/launcher/launcher/pkg/osquery/runtime/osqueryinstance.go",
					"function": "github.com/kolide/launcher/pkg/osquery/runtime.(*OsqueryInstance).Launch.func4",
					"line":     fileLineAsFloat,
				}),
				slog.String("original.component", "osquery_instance"),
				slog.String("original.registration_id", "default"),
				slog.String("original.instance_run_id", "01K0YYVJK0G5A33RSC8VCKMK26"),
				slog.String("original.path", "/usr/local/kolide-nababe-k2/bin/osqueryd"),
				slog.String("err", "signal: killed"),
			},
		},
		{
			testCaseName:       "valid error-level log",
			rawLogRecord:       fmt.Appendf(nil, `{"time":"%s","level":"ERROR","uid":"test","pid":968,"source":{"line":%d,"function":"github.com/kolide/launcher/ee/desktop/runner.(*DesktopUsersProcessesRunner).refreshMenu","file":"D:/a/launcher/launcher/ee/desktop/runner/runner.go"},"msg":"sending refresh command to user desktop process","err":"making request: Get \"http://unix/refresh\": open \\\\.\\pipe\\kolide_desktop_01K0ZRX5V7C2JK4S019TQ8GBB3: The system cannot find the file specified.","path":"C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data\\updates\\launcher\\1.23.1\\launcher.exe","component":"desktop_runner"}`, logTimeString, fileLineInt),
			expectedLogMessage: "sending refresh command to user desktop process",
			expectedLogLevel:   slog.LevelError,
			expectedLogAttributes: []slog.Attr{
				slog.String("original.time", logTimeString),
				slog.Any("original.source", map[string]any{
					"file":     "D:/a/launcher/launcher/ee/desktop/runner/runner.go",
					"function": "github.com/kolide/launcher/ee/desktop/runner.(*DesktopUsersProcessesRunner).refreshMenu",
					"line":     fileLineAsFloat,
				}),
				slog.String("original.component", "desktop_runner"),
				slog.String("original.uid", "test"),
				slog.Float64("original.pid", 968),
				slog.String("err", "making request: Get \"http://unix/refresh\": open \\\\.\\pipe\\kolide_desktop_01K0ZRX5V7C2JK4S019TQ8GBB3: The system cannot find the file specified."),
				slog.String("original.path", "C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data\\updates\\launcher\\1.23.1\\launcher.exe"),
			},
		},
		{
			testCaseName:       "valid runner log",
			rawLogRecord:       fmt.Appendf(nil, `{"time":"%s","level":"INFO","source":{"function":"github.com/kolide/launcher/pkg/rungroup.(*Group).Run","file":"/Users/runner/work/launcher/launcher/pkg/rungroup/rungroup.go","line":%d},"msg":"received interrupt error from first actor -- shutting down other actors","subprocess":"desktop","session_pid":57974,"uid":"501","component":"run_group","err":null,"error_source":"desktopServerShutdownListener"}`, logTimeString, fileLineInt),
			expectedLogMessage: "received interrupt error from first actor -- shutting down other actors",
			expectedLogLevel:   slog.LevelInfo,
			expectedLogAttributes: []slog.Attr{
				slog.String("original.time", logTimeString),
				slog.Any("original.source", map[string]any{
					"file":     "/Users/runner/work/launcher/launcher/pkg/rungroup/rungroup.go",
					"function": "github.com/kolide/launcher/pkg/rungroup.(*Group).Run",
					"line":     fileLineAsFloat,
				}),
				slog.String("original.subprocess", "desktop"),
				slog.Float64("original.session_pid", 57974),
				slog.String("original.uid", "501"),
				slog.String("original.component", "run_group"),
				slog.String("original.error_source", "desktopServerShutdownListener"),
			},
		},
		{
			testCaseName:       "stack trace preserved",
			rawLogRecord:       fmt.Appendf(nil, `{"time":"%s","level":"ERROR","msg":"panic stack trace","source":{"file":"/Users/rebeccamahany-horton/Repos/launcher/ee/gowrapper/goroutine.go","function":"github.com/kolide/launcher/ee/gowrapper.GoWithRecoveryAction.func1.1","line":%d},"subprocess":"desktop","uid":"501","stack_trace":"runtime error: index out of range [4] with length 0\ngithub.com/kolide/launcher/ee/gowrapper.GoWithRecoveryAction.func1.1\n\t/Users/rebeccamahany-horton/Repos/launcher/ee/gowrapper/goroutine.go:31\nruntime.gopanic\n\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/runtime/panic.go:792\nruntime.goPanicIndex\n\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/runtime/panic.go:115\nmain.runDesktop.func3\n\t/Users/rebeccamahany-horton/Repos/launcher/cmd/launcher/desktop.go:135\ngithub.com/kolide/launcher/pkg/rungroup.(*Group).Run.func1\n\t/Users/rebeccamahany-horton/Repos/launcher/pkg/rungroup/rungroup.go:76\ngithub.com/kolide/launcher/ee/gowrapper.GoWithRecoveryAction.func1\n\t/Users/rebeccamahany-horton/Repos/launcher/ee/gowrapper/goroutine.go:39\nruntime.goexit\n\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/runtime/asm_arm64.s:1223","session_pid":46329,"component":"run_group"}`, logTimeString, fileLineInt),
			expectedLogMessage: "panic stack trace",
			expectedLogLevel:   slog.LevelError,
			expectedLogAttributes: []slog.Attr{
				slog.String("original.time", logTimeString),
				slog.Any("original.source", map[string]any{
					"file":     "/Users/rebeccamahany-horton/Repos/launcher/ee/gowrapper/goroutine.go",
					"function": "github.com/kolide/launcher/ee/gowrapper.GoWithRecoveryAction.func1.1",
					"line":     fileLineAsFloat,
				}),
				slog.String("original.component", "run_group"),
				slog.String("original.subprocess", "desktop"),
				slog.Float64("original.session_pid", 46329),
				slog.String("original.uid", "501"),
				slog.String("stack_trace", "runtime error: index out of range [4] with length 0\ngithub.com/kolide/launcher/ee/gowrapper.GoWithRecoveryAction.func1.1\n\t/Users/rebeccamahany-horton/Repos/launcher/ee/gowrapper/goroutine.go:31\nruntime.gopanic\n\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/runtime/panic.go:792\nruntime.goPanicIndex\n\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/runtime/panic.go:115\nmain.runDesktop.func3\n\t/Users/rebeccamahany-horton/Repos/launcher/cmd/launcher/desktop.go:135\ngithub.com/kolide/launcher/pkg/rungroup.(*Group).Run.func1\n\t/Users/rebeccamahany-horton/Repos/launcher/pkg/rungroup/rungroup.go:76\ngithub.com/kolide/launcher/ee/gowrapper.GoWithRecoveryAction.func1\n\t/Users/rebeccamahany-horton/Repos/launcher/ee/gowrapper/goroutine.go:39\nruntime.goexit\n\t/opt/homebrew/Cellar/go/1.24.5/libexec/src/runtime/asm_arm64.s:1223"),
			},
		},
		{
			testCaseName:          "invalid JSON, notification error",
			rawLogRecord:          []byte(`2025-07-29 09:09:01.270 launcher[15047:189395675] Error asking for permission to send notifications Error Domain=UNErrorDomain Code=1 "Notifications are not allowed for this application" UserInfo={NSLocalizedDescription=Notifications are not allowed for this application}`),
			expectedLogMessage:    `2025-07-29 09:09:01.270 launcher[15047:189395675] Error asking for permission to send notifications Error Domain=UNErrorDomain Code=1 "Notifications are not allowed for this application" UserInfo={NSLocalizedDescription=Notifications are not allowed for this application}`,
			expectedLogLevel:      slog.LevelWarn,
			expectedLogAttributes: []slog.Attr{},
		},
		{
			testCaseName:          "invalid JSON, unknown log",
			rawLogRecord:          []byte(`sudo: unable to execute /some/path/to/launcher: Permission denied`),
			expectedLogMessage:    `sudo: unable to execute /some/path/to/launcher: Permission denied`,
			expectedLogLevel:      slog.LevelError,
			expectedLogAttributes: []slog.Attr{},
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// The test handler will receive the incoming log record and assert our expectations against it.
			slogger := slog.New(newTestHandler(t, tt.expectedLogMessage, tt.expectedLogLevel, tt.expectedLogAttributes))

			// Process raw log record and allow the test handler to validate the outcome.
			LogRawLogRecord(t.Context(), tt.rawLogRecord, slogger)
		})
	}
}

type testHandler struct {
	t                       *testing.T
	expectedLogMessage      string
	expectedLogLevel        slog.Level
	expectedLogAttributeMap map[string]any
}

func newTestHandler(t *testing.T, expectedLogMessage string, expectedLogLevel slog.Level, expectedLogAttributes []slog.Attr) *testHandler {
	logAttrMap := make(map[string]any)
	for _, attr := range expectedLogAttributes {
		logAttrMap[attr.Key] = attr.Value.Any()
	}
	return &testHandler{
		t:                       t,
		expectedLogMessage:      expectedLogMessage,
		expectedLogLevel:        expectedLogLevel,
		expectedLogAttributeMap: logAttrMap,
	}
}

func (th *testHandler) Enabled(context.Context, slog.Level) bool {
	// Test handler handles all levels for simplicity's sake
	return true
}

func (th *testHandler) Handle(ctx context.Context, r slog.Record) error {
	require.Equal(th.t, th.expectedLogMessage, r.Message, "msg is incorrect")
	require.Equal(th.t, th.expectedLogLevel, r.Level, "level is incorrect")

	// Check our attrs -- original.time plus everything in th.expectedLogAttributeMap
	foundExpectedAttrs := make([]string, 0)
	r.Attrs(func(a slog.Attr) bool {
		expectedVal, ok := th.expectedLogAttributeMap[a.Key]
		if !ok {
			// Not an attr we were expecting (maybe a slogger attr), no need to check it
			return true
		}
		foundExpectedAttrs = append(foundExpectedAttrs, a.Key)
		require.Equal(th.t, expectedVal, a.Value.Any())
		return true
	})

	require.Equal(th.t, len(th.expectedLogAttributeMap), len(foundExpectedAttrs), fmt.Sprintf("did not find all expected attrs: found: %+v; expected: %+v", foundExpectedAttrs, th.expectedLogAttributeMap))

	return nil
}

func (th *testHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// No-op for test
	return th
}

func (th *testHandler) WithGroup(name string) slog.Handler {
	// No-op for test
	return th
}
