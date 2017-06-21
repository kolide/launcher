all: build

.PHONY: build
build: agent extension

.pre-build:
	mkdir -p build

extension: .pre-build
	go build -o build/osquery-extension.ext ./cmd/osquery-extension/

agent: .pre-build
	go build -o build/agent ./cmd/agent/

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure

test:
	go test ./cmd/...
	go test ./osquery
