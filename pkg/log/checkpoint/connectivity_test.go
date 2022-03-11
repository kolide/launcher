package checkpoint

import (
	"errors"
	"net"
	"net/url"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
)

func Test_testConnections(t *testing.T) {
	t.Parallel()

	type args struct {
		dialer *mocks.Dialer
		urls   []*url.URL
	}
	tests := []struct {
		name          string
		args          args
		onDialReturns []func() (net.Conn, error)
		want          map[string]string
	}{
		{
			name: "happy_path",
			args: args{
				dialer: &mocks.Dialer{},
				urls: []*url.URL{
					{
						Host: "happy_path_1.example.com",
					},
					{
						Host: "happy_path_2.example.com",
					},
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
			want: map[string]string{
				"happy_path_1.example.com": "successful tcp connection",
				"happy_path_2.example.com": "successful tcp connection",
			},
		},
		{
			name: "error",
			args: args{
				dialer: &mocks.Dialer{},
				urls: []*url.URL{
					{
						Host: "error_1.example.com",
					},
					{
						Host: "error_2.example.com",
					},
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
			want: map[string]string{
				"error_1.example.com": "some error",
				"error_2.example.com": "some error",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for i, url := range tt.args.urls {
				tt.args.dialer.On("Dial", "tcp", url.Host).Return(tt.onDialReturns[i]())
			}

			if got := testConnections(tt.args.dialer, tt.args.urls...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("testConnections() = %v, want %v", got, tt.want)
			}

			tt.args.dialer.AssertExpectations(t)
		})
	}
}
