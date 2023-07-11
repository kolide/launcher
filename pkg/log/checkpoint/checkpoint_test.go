package checkpoint

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/agent/types/mocks"
)

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
