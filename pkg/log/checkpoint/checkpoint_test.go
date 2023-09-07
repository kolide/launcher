package checkpoint

import (
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestInterrupt(t *testing.T) {
	t.Parallel()

	// Set up temp db
	file, err := os.CreateTemp("", "kolide_launcher_test")
	if err != nil {
		t.Fatalf("creating temp file: %s", err.Error())
	}

	db, err := bbolt.Open(file.Name(), 0600, nil)
	if err != nil {
		t.Fatalf("opening bolt DB: %s", err.Error())
	}

	defer func() {
		db.Close()
		os.Remove(file.Name())
	}()

	// Set up knapsack
	k := mocks.NewKnapsack(t)
	k.On("BboltDB").Return(db)
	k.On("KolideHosted").Return(true)
	k.On("InModernStandby").Return(false).Maybe()
	k.On("KolideServerURL").Return("")
	k.On("InsecureTransportTLS").Return(false)
	k.On("Autoupdate").Return(true)
	k.On("MirrorServerURL").Return("")
	k.On("NotaryServerURL").Return("")
	k.On("TufServerURL").Return("")
	k.On("ControlServerURL").Return("")

	// Start the checkpointer, let it run, interrupt it, and confirm it can return from the interrupt
	testCheckpointer := New(log.NewNopLogger(), k)

	runInterruptReceived := make(chan struct{}, 1)

	go func() {
		require.Nil(t, testCheckpointer.Run())
		runInterruptReceived <- struct{}{}
	}()

	// Give it a couple seconds to run before calling interrupt
	time.Sleep(3 * time.Second)

	testCheckpointer.Interrupt(nil)

	select {
	case <-runInterruptReceived:
		break
	case <-time.After(5 * time.Second):
		t.Error("could not interrupt checkpointer within 5 seconds")
		t.FailNow()
	}

	// Now call interrupt a couple more times
	expectedAdditionalInterrupts := 3
	additionalInterruptsReceived := make(chan struct{}, expectedAdditionalInterrupts)

	for i := 0; i < expectedAdditionalInterrupts; i += 1 {
		go func() {
			testCheckpointer.Interrupt(nil)
			additionalInterruptsReceived <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedAdditionalInterrupts {
			break
		}

		select {
		case <-additionalInterruptsReceived:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedAdditionalInterrupts, receivedInterrupts)
}

func Test_urlsToTest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mock func(t *testing.T) *mocks.Flags
		want []*url.URL
	}{
		{
			name: "kolide_saas",
			mock: func(t *testing.T) *mocks.Flags {
				m := mocks.NewFlags(t)
				m.On("InsecureTransportTLS").Return(false)
				m.On("KolideServerURL").Return("k2device.kolide.com:443")
				m.On("ControlServerURL").Return("k2control.kolide.com:443")
				m.On("Autoupdate").Return(true)
				m.On("MirrorServerURL").Return("dl.kolide.co:443")
				m.On("NotaryServerURL").Return("notary.kolide.co:443")
				m.On("TufServerURL").Return("tuf.kolide.com:443")
				return m
			},
			want: []*url.URL{
				{
					Host:   "k2device.kolide.com:443",
					Scheme: "https",
				},
				{
					Host:   "dl.kolide.co:443",
					Scheme: "https",
				},
				{
					Host:   "notary.kolide.co:443",
					Scheme: "https",
				},
				{
					Host:   "tuf.kolide.com:443",
					Scheme: "https",
				},
				{
					Host:   "k2control.kolide.com:443",
					Scheme: "https",
				},
			},
		},
		{
			name: "no_control",
			mock: func(t *testing.T) *mocks.Flags {
				m := mocks.NewFlags(t)
				m.On("InsecureTransportTLS").Return(false)
				m.On("KolideServerURL").Return("k2device.kolide.com:443")
				m.On("ControlServerURL").Return("")
				m.On("Autoupdate").Return(true)
				m.On("MirrorServerURL").Return("dl.kolide.co:443")
				m.On("NotaryServerURL").Return("notary.kolide.co:443")
				m.On("TufServerURL").Return("tuf.kolide.com:443")
				return m
			},
			want: []*url.URL{
				{
					Host:   "k2device.kolide.com:443",
					Scheme: "https",
				},
				{
					Host:   "dl.kolide.co:443",
					Scheme: "https",
				},
				{
					Host:   "notary.kolide.co:443",
					Scheme: "https",
				},
				{
					Host:   "tuf.kolide.com:443",
					Scheme: "https",
				},
			},
		},
		{
			name: "no_autoupdate",
			mock: func(t *testing.T) *mocks.Flags {
				m := mocks.NewFlags(t)
				m.On("InsecureTransportTLS").Return(false)
				m.On("KolideServerURL").Return("k2device.kolide.com:443")
				m.On("ControlServerURL").Return("k2control.kolide.com:443")
				m.On("Autoupdate").Return(false)
				return m
			},
			want: []*url.URL{
				{
					Host:   "k2device.kolide.com:443",
					Scheme: "https",
				},
				{
					Host:   "k2control.kolide.com:443",
					Scheme: "https",
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := urlsToTest(tt.mock(t))

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("urlsToTest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseUrl(t *testing.T) {
	t.Parallel()

	type args struct {
		addr string
	}
	tests := []struct {
		name              string
		insecureTransport bool
		args              args
		want              *url.URL
		wantErr           bool
		portDefined       bool
	}{
		{
			name: "secure_with_port_input",
			args: args{
				addr: "example.com:443",
			},
			want: &url.URL{
				Host:   "example.com:443",
				Scheme: "https",
			},
		},
		{
			name: "secure_no_port_input",
			args: args{
				addr: "example.com",
			},
			want: &url.URL{
				Host:   "example.com:443",
				Scheme: "https",
			},
		},
		{
			name:              "insecure_with_port_input",
			insecureTransport: true,
			args: args{
				addr: "example.com:80",
			},
			want: &url.URL{
				Host:   "example.com:80",
				Scheme: "http",
			},
		},
		{
			name:              "insecure_no_port_input",
			insecureTransport: true,
			args: args{
				addr: "example.com",
			},
			want: &url.URL{
				Host:   "example.com:80",
				Scheme: "http",
			},
		},
		{
			name: "addr_with_scheme",
			args: args{
				addr: "https://example.com",
			},
			want: &url.URL{
				Host:   "example.com:443",
				Scheme: "https",
			},
		},
		{
			name:        "addr_with_scheme_and_port",
			portDefined: true,
			args: args{
				addr: "https://example.com:443",
			},
			want: &url.URL{
				Host:   "example.com:443",
				Scheme: "https",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockFlags := mocks.NewFlags(t)
			if !tt.portDefined {
				mockFlags.On("InsecureTransportTLS").Return(tt.insecureTransport)
			}

			got, err := parseUrl(tt.args.addr, mockFlags)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseUrl() = %v, want %v", got, tt.want)
			}
		})
	}
}
