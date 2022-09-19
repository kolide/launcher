package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
	"github.com/stretchr/testify/mock"
)

func Test_lookupHostsIpv4s(t *testing.T) {
	t.Parallel()

	type args struct {
		ipLookuper *mocks.IpLookuper
		urls       []*url.URL
	}
	tests := []struct {
		name              string
		args              args
		onLookupIPReturns []func() ([]net.IP, error)
		want              map[string]interface{}
	}{
		{
			name: "happy_path_single",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				urls: []*url.URL{
					{
						Host:   "happy_path_single.example.com",
						Scheme: "https",
					},
				},
			},
			onLookupIPReturns: []func() ([]net.IP, error){
				func() ([]net.IP, error) {
					return []net.IP{
						net.ParseIP("192.0.0.0"),
						net.ParseIP("2001:db8::68"),
					}, nil
				},
			},
			want: map[string]interface{}{
				"happy_path_single.example.com": []string{"192.0.0.0"},
			},
		},
		{
			name: "happy_path_multiple",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				urls: []*url.URL{
					{
						Host: "happy_path_multiple_1.example.com",
					},
					{
						Host: "happy_path_multiple_2.example.com",
					},
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
			want: map[string]interface{}{
				"happy_path_multiple_1.example.com": []string{"192.0.0.0", "192.0.0.1"},
				"happy_path_multiple_2.example.com": []string{"192.0.1.0"},
			},
		},
		{
			name: "error",
			args: args{
				ipLookuper: &mocks.IpLookuper{},
				urls: []*url.URL{
					{
						Host: "happy_path_multiple_1.example.com",
					},
					{
						Host: "error.example.com",
					},
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
			want: map[string]interface{}{
				"happy_path_multiple_1.example.com": []string{"192.0.0.0", "192.0.0.1"},
				"error.example.com":                 "some error",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for i, url := range tt.args.urls {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				defer cancel()
				tt.args.ipLookuper.On("LookupIP", mock.AnythingOfType(fmt.Sprintf("%T", ctx)), "ip", url.Host).Return(tt.onLookupIPReturns[i]())
			}

			if got := lookupHostsIpv4s(tt.args.ipLookuper, tt.args.urls...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lookupIpv4Log() = %v, want %v", got, tt.want)
			}

			tt.args.ipLookuper.AssertExpectations(t)
		})
	}
}
