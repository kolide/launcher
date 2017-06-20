all: build

.PHONY: build
build: agent extproxy

.pre-build:
	mkdir -p build

extproxy: .pre-build
	go build -o build/extproxy.ext ./cmd/extproxy/

agent: .pre-build
	go build -o build/agent ./cmd/agent/

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure

test:
	go test ./cmd/...
	go test ./osquery
