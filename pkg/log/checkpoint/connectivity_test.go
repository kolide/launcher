package checkpoint

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
)

func Test_testConnections(t *testing.T) {
	t.Parallel()

	type args struct {
		dialer *mocks.Dialer
		hosts  []string
	}
	tests := []struct {
		name          string
		args          args
		onDialReturns []func() (net.Conn, error)
		want          []string
	}{
		{
			name: "happy_path",
			args: args{
				dialer: &mocks.Dialer{},
				hosts: []string{
					"happy_path_1",
					"happy_path_2",
				},
			},
			onDialReturns: []func() (net.Conn, error){
				func() (net.Conn, error) {
					return &net.TCPConn{}, nil
				},
				func() (net.Conn, error) {
					return &net.TCPConn{}, nil
				},
			},
			want: []string{
				"happy_path_1: success",
				"happy_path_2: success",
			},
		},
		{
			name: "error",
			args: args{
				dialer: &mocks.Dialer{},
				hosts: []string{
					"error_1",
					"error_2",
				},
			},
			onDialReturns: []func() (net.Conn, error){
				func() (net.Conn, error) {
					return nil, errors.New("some error")
				},
				func() (net.Conn, error) {
					return nil, errors.New("some error")
				},
			},
			want: []string{
				"error_1: some error",
				"error_2: some error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, host := range tt.args.hosts {
				tt.args.dialer.On("Dial", "tcp", net.JoinHostPort(host, "443")).Return(tt.onDialReturns[i]())
			}

			if got := testConnections(tt.args.dialer, tt.args.hosts...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("testConnections() = %v, want %v", got, tt.want)
			}

			tt.args.dialer.AssertExpectations(t)
		})
	}
}
