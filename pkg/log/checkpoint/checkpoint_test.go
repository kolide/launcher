package checkpoint

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/launcher"
)

func Test_urlsToTest(t *testing.T) {
	t.Parallel()

	type args struct {
		opts launcher.Options
	}
	tests := []struct {
		name string
		args args
		want []*url.URL
	}{
		{
			name: "kolide_saas",
			args: args{
				opts: launcher.Options{
					KolideServerURL:  "k2device.kolide.com:443",
					Control:          true,
					ControlServerURL: "k2control.kolide.com:443",
					Autoupdate:       true,
					NotaryServerURL:  "notary.kolide.co:443",
					TufServerURL:     "tuf.kolide.com:443",
					MirrorServerURL:  "dl.kolide.co:443",
				},
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
			args: args{
				opts: launcher.Options{
					KolideServerURL: "k2device.kolide.com:443",
					Autoupdate:      true,
					NotaryServerURL: "notary.kolide.co:443",
					TufServerURL:    "tuf.kolide.com:443",
					MirrorServerURL: "dl.kolide.co:443",
				},
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
			args: args{
				opts: launcher.Options{
					KolideServerURL:  "k2device.kolide.com:443",
					Control:          true,
					ControlServerURL: "k2control.kolide.com:443",
					NotaryServerURL:  "notary.kolide.co:443",
					TufServerURL:     "tuf.kolide.com:443",
					MirrorServerURL:  "dl.kolide.co:443",
				},
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

			mockFlags := mocks.NewFlags(t)

			got := urlsToTest(mockFlags)

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
		opts launcher.Options
	}
	tests := []struct {
		name    string
		args    args
		want    *url.URL
		wantErr bool
	}{
		{
			name: "secure_with_port_input",
			args: args{
				addr: "example.com:443",
				opts: launcher.Options{},
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
				opts: launcher.Options{},
			},
			want: &url.URL{
				Host:   "example.com:443",
				Scheme: "https",
			},
		},
		{
			name: "insecure_with_port_input",
			args: args{
				addr: "example.com:80",
				opts: launcher.Options{
					InsecureTransport: true,
				},
			},
			want: &url.URL{
				Host:   "example.com:80",
				Scheme: "http",
			},
		},
		{
			name: "insecure_no_port_input",
			args: args{
				addr: "example.com",
				opts: launcher.Options{
					InsecureTransport: true,
				},
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
				opts: launcher.Options{},
			},
			want: &url.URL{
				Host:   "example.com:443",
				Scheme: "https",
			},
		},
		{
			name: "addr_with_scheme_and_port",
			args: args{
				addr: "https://example.com:443",
				opts: launcher.Options{},
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
