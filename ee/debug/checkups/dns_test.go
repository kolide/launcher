package checkups

import (
	"errors"
	"io"
	"strings"
	"testing"

	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/debug/checkups/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_dnsCheckup_Run(t *testing.T) {
	t.Parallel()
	type fields struct {
		k        *typesMocks.Knapsack
		status   Status
		summary  string
		data     map[string]any
		resolver *mocks.HostResolver
	}
	type resolution struct {
		ips []string
		err error
	}
	tests := []struct {
		name                  string
		fields                fields
		knapsackReturns       map[string]any
		onLookupHostReturns   map[string]resolution
		expectedStatus        Status
		expectedSuccessCount  int
		expectedAttemptsCount int
	}{
		{
			name: "happy path",
			fields: fields{
				k:        typesMocks.NewKnapsack(t),
				resolver: mocks.NewHostResolver(t),
			},
			knapsackReturns: map[string]interface{}{
				"KolideServerURL":      "https://kolide-server.example.com",
				"ControlServerURL":     "https://control-server.example.com",
				"TufServerURL":         "https://tuf-server.example.com",
				"InsecureTransportTLS": false,
			},
			onLookupHostReturns: map[string]resolution{
				"kolide-server.example.com":  {ips: []string{"2607:f8b0:4009:808::200e", "142.250.65.206"}, err: nil},
				"control-server.example.com": {ips: []string{"111.2.3.4", "111.2.3.5"}, err: nil},
				"tuf-server.example.com":     {ips: []string{"34.149.84.181"}, err: nil},
				"google.com":                 {ips: []string{"2620:149:af0::10", "17.253.144.10"}, err: nil},
				"apple.com":                  {ips: []string{"2607:f8b0:4009:808::200e", "142.250.65.206"}, err: nil},
			},
			expectedStatus:        Passing,
			expectedSuccessCount:  5,
			expectedAttemptsCount: 5,
		},
		{
			name: "unresolvable host errors are included in data",
			fields: fields{
				k:        typesMocks.NewKnapsack(t),
				resolver: mocks.NewHostResolver(t),
			},
			knapsackReturns: map[string]interface{}{
				"KolideServerURL":      "https://kolide-server.example.com",
				"ControlServerURL":     "https://control-server.example.com",
				"TufServerURL":         "https://tuf-server.example.com",
				"InsecureTransportTLS": false,
			},
			onLookupHostReturns: map[string]resolution{
				"kolide-server.example.com":  {ips: []string{}, err: errors.New("Unable to resolve: No Such Host")},
				"control-server.example.com": {ips: []string{"111.2.3.4", "111.2.3.5"}, err: nil},
				"tuf-server.example.com":     {ips: []string{"34.149.84.181"}, err: nil},
				"google.com":                 {ips: []string{"2620:149:af0::10", "17.253.144.10"}, err: nil},
				"apple.com":                  {ips: []string{"2607:f8b0:4009:808::200e", "142.250.65.206"}, err: nil},
			},
			expectedStatus:        Warning,
			expectedSuccessCount:  4,
			expectedAttemptsCount: 5,
		},
		{
			name: "unset host values are not counted against checkup",
			fields: fields{
				k:        typesMocks.NewKnapsack(t),
				resolver: mocks.NewHostResolver(t),
			},
			knapsackReturns: map[string]interface{}{
				"KolideServerURL":      "https://kolide-server.example.com",
				"ControlServerURL":     "https://control-server.example.com",
				"TufServerURL":         "",
				"InsecureTransportTLS": false,
			},
			onLookupHostReturns: map[string]resolution{
				"kolide-server.example.com":  resolution{ips: []string{"2607:f8b0:4009:808::200e", "142.250.65.206"}, err: nil},
				"control-server.example.com": resolution{ips: []string{"111.2.3.4", "111.2.3.5"}, err: nil},
				"google.com":                 resolution{ips: []string{"2620:149:af0::10", "17.253.144.10"}, err: nil},
				"apple.com":                  resolution{ips: []string{"2607:f8b0:4009:808::200e", "142.250.65.206"}, err: nil},
			},
			expectedStatus:        Passing,
			expectedSuccessCount:  4,
			expectedAttemptsCount: 4,
		},
	}

	for _, tt := range tests {
		tt := tt
		for mockfunc, mockval := range tt.knapsackReturns {
			tt.fields.k.On(mockfunc).Return(mockval)
		}

		for mockhost, mockresolution := range tt.onLookupHostReturns {
			tt.fields.resolver.On("LookupHost", mock.Anything, mockhost).Return(mockresolution.ips, mockresolution.err).Once()
		}

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dc := &dnsCheckup{
				k:        tt.fields.k,
				status:   tt.fields.status,
				summary:  tt.fields.summary,
				data:     tt.fields.data,
				resolver: tt.fields.resolver,
			}
			if err := dc.Run(t.Context(), io.Discard); err != nil {
				t.Errorf("dnsCheckup.Run() error = %v", err)
				return
			}

			gotData, ok := dc.Data().(map[string]any)
			require.True(t, ok, "expected to be able to typecast data into map[string]any for testing")

			for mockhost, mockresolution := range tt.onLookupHostReturns {
				if mockresolution.err != nil {
					require.Contains(t, gotData[mockhost], mockresolution.err.Error(), "expected data for %s to contain the resolution error message", mockhost)
				} else {
					require.Contains(t, gotData[mockhost], strings.Join(mockresolution.ips, ","), "expected data to contain the resulting IP addresses")
				}
			}

			require.Equal(t, tt.expectedAttemptsCount, gotData["lookup_attempts"], "expected lookup attempts to match")
			require.Equal(t, tt.expectedSuccessCount, gotData["lookup_successes"], "expected lookup success count to match")
		})
	}
}
