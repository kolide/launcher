package checkpoint

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
)

func Test_fetchFromUrls(t *testing.T) {
	t.Parallel()

	type args struct {
		client *mocks.HttpClient
		urls   []string
	}
	tests := []struct {
		name         string
		args         args
		onGetReturns []func() (*http.Response, error)
		want         []string
	}{
		{
			name: "happy_path_single",
			args: args{
				client: &mocks.HttpClient{},
				urls:   []string{"https://happy_path_single.com"},
			},
			onGetReturns: []func() (*http.Response, error){
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
						Body:   io.NopCloser(bytes.NewBufferString("happy_path_response_single")),
					}, nil
				},
			},
			want: []string{"https://happy_path_single.com: [200 OK] happy_path_response_single"},
		},
		{
			name: "happy_path_multiple",
			args: args{
				client: &mocks.HttpClient{},
				urls:   []string{"https://happy_path_multiple_1.com", "https://happy_path_multiple_2.com"},
			},
			onGetReturns: []func() (*http.Response, error){
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
						Body:   io.NopCloser(bytes.NewBufferString("happy_path_response_multiple_1")),
					}, nil
				},
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
						Body:   io.NopCloser(bytes.NewBufferString("happy_path_response_multiple_2")),
					}, nil
				},
			},
			want: []string{"https://happy_path_multiple_1.com: [200 OK] happy_path_response_multiple_1", "https://happy_path_multiple_2.com: [200 OK] happy_path_response_multiple_2"},
		},
		{
			name: "error",
			args: args{
				client: &mocks.HttpClient{},
				urls:   []string{"https://error.com", "https://happy_path_multiple_2.com"},
			},
			onGetReturns: []func() (*http.Response, error){
				func() (*http.Response, error) {
					return nil, errors.New("some error")
				},
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
						Body:   io.NopCloser(bytes.NewBufferString("happy_path_response_multiple_2")),
					}, nil
				},
			},
			want: []string{"https://error.com: some error", "https://happy_path_multiple_2.com: [200 OK] happy_path_response_multiple_2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, url := range tt.args.urls {
				tt.args.client.On("Get", url).Return(tt.onGetReturns[i]()) //nolint:bodyclose
			}
			if got := fetchFromUrls(tt.args.client, tt.args.urls); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchFromUrls() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_fetchNotaryVersion(t *testing.T) {
	type args struct {
		client *mocks.HttpClient
		url    string
	}
	tests := []struct {
		name        string
		args        args
		onGetReturn func() (*http.Response, error)
		want        string
	}{
		{
			name: "happy_path",
			args: args{
				client: &mocks.HttpClient{},
				url:    "https://happy_path.com",
			},
			onGetReturn: func() (*http.Response, error) {
				return &http.Response{
					Body: io.NopCloser(bytes.NewBufferString(`
					{
						"signed": {
							"version": 1516
						},
						"signatures": [
							{
								"keyid": "e9f1ebbbacfbcfa7663a11fee95d634ae599d2c583ebdc2bbecc41ee4414c1a4",
								"method": "ecdsa"
							}
						]
					}`)),
				}, nil
			},
			want: "https://happy_path.com: 1516",
		},
		{
			name: "error",
			args: args{
				client: &mocks.HttpClient{},
				url:    "https://error.com",
			},
			onGetReturn: func() (*http.Response, error) {
				return nil, errors.New("some error")
			},
			want: "https://error.com: some error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.client.On("Get", tt.args.url).Return(tt.onGetReturn()) //nolint:bodyclose
			if got := fetchNotaryVersion(tt.args.client, tt.args.url); got != tt.want {
				t.Errorf("fetchNotaryVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
