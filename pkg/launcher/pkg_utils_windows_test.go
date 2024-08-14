//go:build windows
// +build windows

package launcher

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ServiceName(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName        string
		identifier          string
		expectedServiceName string
	}{
		{
			testCaseName:        "empty identifier expecting default service name",
			identifier:          " ",
			expectedServiceName: "LauncherKolideK2Svc",
		},
		{
			testCaseName:        "default identifier expecting default service name",
			identifier:          "kolide-k2",
			expectedServiceName: "LauncherKolideK2Svc",
		},
		{
			testCaseName:        "preprod identifier expecting preprod service name",
			identifier:          "kolide-preprod-k2",
			expectedServiceName: "LauncherKolidePreprodK2Svc",
		},
		{
			testCaseName:        "mangled identifier expecting default service name",
			identifier:          "kolide-!@_k2",
			expectedServiceName: "LauncherKolideK2Svc",
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			serviceName := ServiceName(tt.identifier)
			require.Equal(t, tt.expectedServiceName, serviceName, "expected sanitized service name value to match")
		})
	}
}
