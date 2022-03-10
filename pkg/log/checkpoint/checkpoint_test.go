package checkpoint

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/launcher"
)

func Test_urlsToTest(t *testing.T) {
	t.Parallel()

	type args struct {
		opts launcher.Options
	}
	tests := []struct {
		name    string
		args    args
		want    []*url.URL
		wantErr bool
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
		t.Run(tt.name, func(t *testing.T) {
			got, err := urlsToTest(tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("urlsToTest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("urlsToTest() = %v, want %v", got, tt.want)
			}
		})
	}
}
