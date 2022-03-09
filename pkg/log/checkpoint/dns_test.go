package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
	"github.com/stretchr/testify/mock"
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
		want              map[string]interface{}
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
			want: map[string]interface{}{"happy_path_single.com": []string{"192.0.0.0"}},
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
			want: map[string]interface{}{
				"happy_path_multiple_1.com": []string{"192.0.0.0", "192.0.0.1"},
				"happy_path_multiple_2.com": []string{"192.0.1.0"},
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
			want: map[string]interface{}{
				"happy_path_multiple_1.com": []string{"192.0.0.0", "192.0.0.1"},
				"error.com":                 "some error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, host := range tt.args.hosts {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				defer cancel()
				tt.args.ipLookuper.On("LookupIP", mock.AnythingOfType(fmt.Sprintf("%T", ctx)), "ip", host).Return(tt.onLookupIPReturns[i]())
			}

			if got := lookupHostsIpv4s(tt.args.ipLookuper, tt.args.hosts...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lookupIpv4Log() = %v, want %v", got, tt.want)
			}

			tt.args.ipLookuper.AssertExpectations(t)
		})
	}
}
