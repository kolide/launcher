package checkpoint

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
)

func Test_lookupHostsIpv4s(t *testing.T) {
	t.Parallel()

	type args struct {
		ipLookuper *mocks.IpLookuper
		hosts      []string
	}
	tests := []struct {
		name              string
		args              args
		onLookupIPReturns []func() ([]net.IP, error)
		want              []string
	}{
		{
			name: "happy_path_single",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				hosts:      []string{"happy_path_single.com"},
			},
			onLookupIPReturns: []func() ([]net.IP, error){
				func() ([]net.IP, error) {
					return []net.IP{

						net.ParseIP("192.0.0.0"),
						net.ParseIP("2001:db8::68"),
					}, nil
				},
			},
			want: []string{"happy_path_single.com: 192.0.0.0"},
		},
		{
			name: "happy_path_multiple",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				hosts: []string{
					"happy_path_multiple_1.com",
					"happy_path_multiple_2.com",
				},
			},
			onLookupIPReturns: []func() ([]net.IP, error){
				func() ([]net.IP, error) {
					return []net.IP{
						net.ParseIP("192.0.0.0"),
						net.ParseIP("192.0.0.1"),
						net.ParseIP("2001:db8::68"),
					}, nil
				},
				func() ([]net.IP, error) {
					return []net.IP{
						net.ParseIP("192.0.1.0"),
					}, nil
				},
			},
			want: []string{
				"happy_path_multiple_1.com: 192.0.0.0, 192.0.0.1",
				"happy_path_multiple_2.com: 192.0.1.0",
			},
		},
		{
			name: "error",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				hosts: []string{
					"happy_path_multiple_1.com",
					"error.com",
				},
			},
			onLookupIPReturns: []func() ([]net.IP, error){
				func() ([]net.IP, error) {
					return []net.IP{
						net.ParseIP("192.0.0.0"),
						net.ParseIP("192.0.0.1"),
						net.ParseIP("2001:db8::68"),
					}, nil
				},
				func() ([]net.IP, error) {
					return nil, errors.New("some error")
				},
			},
			want: []string{
				"happy_path_multiple_1.com: 192.0.0.0, 192.0.0.1",
				"error.com: some error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, host := range tt.args.hosts {
				tt.args.ipLookuper.On("LookupIP", context.Background(), "ip", host).Return(tt.onLookupIPReturns[i]())
			}

			if got := lookupHostsIpv4s(tt.args.ipLookuper, tt.args.hosts...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lookupIpv4Log() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_lookupIpv4(t *testing.T) {
	t.Parallel()

	type args struct {
		ipLookuper *mocks.IpLookuper
		host       string
	}
	tests := []struct {
		name             string
		args             args
		onLookupIPReturn func() ([]net.IP, error)
		want             []string
		wantErr          bool
	}{
		{
			name: "happy_path_single",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				host:       "happy_path_single.com",
			},
			onLookupIPReturn: func() ([]net.IP, error) {
				return []net.IP{
					net.ParseIP("192.0.0.0"),
					net.ParseIP("2001:db8::68"),
				}, nil
			},
			want: []string{
				"192.0.0.0",
			},
			wantErr: false,
		},
		{
			name: "happy_path_multiple",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				host:       "happy_path_multiple.com",
			},
			onLookupIPReturn: func() ([]net.IP, error) {
				return []net.IP{
					net.ParseIP("192.0.0.0"),
					net.ParseIP("192.0.0.1"),
					net.ParseIP("2001:db8::68"),
				}, nil
			},
			want: []string{
				"192.0.0.0",
				"192.0.0.1",
			},
			wantErr: false,
		},
		{
			name: "error",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				host:       "error.com",
			},
			onLookupIPReturn: func() ([]net.IP, error) {
				return nil, errors.New("some error")
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.ipLookuper.On("LookupIP", context.Background(), "ip", tt.args.host).Return(tt.onLookupIPReturn())

			got, err := lookupIpv4(tt.args.ipLookuper, tt.args.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("lookupIpv4() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lookupIpv4() = %v, want %v", got, tt.want)
			}
		})
	}
}
