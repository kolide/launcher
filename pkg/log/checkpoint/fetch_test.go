package checkpoint

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/log/checkpoint/mocks"
)

func Test_fetchFromUrls(t *testing.T) {
	t.Parallel()

	type args struct {
		client *mocks.HttpClient
		urls   []*url.URL
	}
	tests := []struct {
		name         string
		args         args
		onGetReturns []func() (*http.Response, error)
		want         map[string]string
	}{
		{
			name: "happy_path_single",
			args: args{
				client: &mocks.HttpClient{},
				urls: []*url.URL{
					{
						Host:   "happy_path_single.example.com",
						Scheme: "https",
					},
				},
			},
			onGetReturns: []func() (*http.Response, error){
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
						Body:   io.NopCloser(bytes.NewBufferString("happy_path_response_single")),
					}, nil
				},
			},
			want: map[string]string{
				"https://happy_path_single.example.com": "200 OK happy_path_response_single",
			},
		},
		{
			name: "happy_path_multiple",
			args: args{
				client: &mocks.HttpClient{},
				urls: []*url.URL{
					{
						Host:   "happy_path_multiple_1.example.com",
						Scheme: "https",
					},
					{
						Host:   "happy_path_multiple_2.example.com",
						Scheme: "https",
					},
				},
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
			want: map[string]string{
				"https://happy_path_multiple_1.example.com": "200 OK happy_path_response_multiple_1",
				"https://happy_path_multiple_2.example.com": "200 OK happy_path_response_multiple_2",
			},
		},
		{
			name: "error",
			args: args{
				client: &mocks.HttpClient{},
				urls: []*url.URL{
					{
						Host:   "error.example.com",
						Scheme: "https",
					},
					{
						Host:   "happy_path_multiple_2.example.com",
						Scheme: "https",
					},
				},
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
			want: map[string]string{
				"https://error.example.com":                 "some error",
				"https://happy_path_multiple_2.example.com": "200 OK happy_path_response_multiple_2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, url := range tt.args.urls {
				tt.args.client.On("Get", url.String()).Return(tt.onGetReturns[i]()) //nolint:bodyclose
			}
			if got := fetchFromUrls(tt.args.client, tt.args.urls...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchFromUrls() = %v, want %v", got, tt.want)
			}
			tt.args.client.AssertExpectations(t)
		})
	}
}

func Test_fetchNotaryVersions(t *testing.T) {
	type args struct {
		client *mocks.HttpClient
		urls   []*url.URL
	}
	tests := []struct {
		name         string
		args         args
		onGetReturns []func() (*http.Response, error)
		want         map[string]string
	}{
		{
			name: "happy_path",
			args: args{
				client: &mocks.HttpClient{},
				urls: []*url.URL{
					{
						Host:   "happy_path_1.example.com",
						Scheme: "https",
					},
					{
						Host:   "happy_path_2.example.com",
						Scheme: "https",
					},
				},
			},
			onGetReturns: []func() (*http.Response, error){
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
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
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
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
			},
			want: map[string]string{
				"https://happy_path_1.example.com": "1516",
				"https://happy_path_2.example.com": "1516",
			},
		},
		{
			name: "error",
			args: args{
				client: &mocks.HttpClient{},
				urls: []*url.URL{
					{
						Host:   "happy_path_1.example.com",
						Scheme: "https",
					},
					{
						Host:   "error.example.com",
						Scheme: "https",
					},
				},
			},
			onGetReturns: []func() (*http.Response, error){
				func() (*http.Response, error) {
					return &http.Response{
						Status: "200 OK",
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
				func() (*http.Response, error) {
					return nil, errors.New("some error")
				},
			},
			want: map[string]string{
				"https://happy_path_1.example.com": "1516",
				"https://error.example.com":        "some error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, url := range tt.args.urls {
				tt.args.client.On("Get", url.String()).Return(tt.onGetReturns[i]()) //nolint:bodyclose
			}

			if got := fetchNotaryVersions(tt.args.client, tt.args.urls...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchNotaryVersion() = %v, want %v", got, tt.want)
			}

			tt.args.client.AssertExpectations(t)
		})
	}
}
