# This is a Dockerfile to _build_ launcher. It's expected that most
# usage will us multi-stage builds, with this as stage1. See the
# docker/ directory and associated Make targets.
#
# Note that multistage builds can leverage the tag applied (at build
# time) to this container

FROM golang:1.12
LABEL maintainer="engineering@kolide.co"

# fake data or not?
ARG FAKE

# The launcher build is generally not GOPATH, however, we do assume
# that the notary files are there. Eg, we hardcode paths.
# Look for `notaryConfigDir`
WORKDIR /go/src/github.com/kolide/launcher

# copy source into the docker builder. Perhaps this would be cleaner
# via a volume mount. But, COPY allows the docker container to have
# zero impact on the host's build directory. This uses .dockerignore
# to exclude the build directory
COPY . ./

# Build!
RUN make deps
RUN make all
RUN GO111MODULE=on go run cmd/make/make.go -targets=launcher,extension -linkstamp $FAKE

# Install
RUN mkdir -p /usr/local/kolide/bin/
RUN cp build/linux/* /usr/local/kolide/bin/
RUN GO111MODULE=on go run ./tools/download-osquery.go  --platform linux --output /usr/local/kolide/bin/osqueryd

# Set entrypoint
ENTRYPOINT ["/usr/local/kolide/bin/launcher"]
CMD []
