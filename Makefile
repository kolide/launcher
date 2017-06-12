all: build

build:
	mkdir -p build
	go build -o build/agent

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure

test:
	go test ./osquery
