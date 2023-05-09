# This is a Dockerfile to _build_ launcher. It's expected that most
# usage will us multi-stage builds, with this as stage1. See the
# docker/ directory and associated Make targets.
#
# Note that multistage builds can leverage the tag applied (at build
# time) to this container

FROM golang:1.20 AS golauncherbuild
LABEL maintainer="engineering@kolide.co"

# fake data or not?
ARG FAKE

# Default version to build
ARG gitver=main

# The launcher build is generally not GOPATH, however, we do assume
# that the notary files are there. Eg, we hardcode paths.
# Look for `notaryConfigDir`
# WORKDIR /go/src/github.com/kolide/launcher

# We need the launcher source to build launcher. We can get this one
# of three ways. git clone, copy, or mount. git clone was chosen, as
# it's the least bad.
# COPY is slow, and can pull in cruft from the working dir
# mount can pull in cruft from the working dir, and can pollute the working dir
# git clone ensures clean source
RUN git clone https://github.com/kolide/launcher.git
RUN cd launcher && git checkout "${gitver}"

# Build!
RUN cd launcher && make deps
RUN cd launcher && make all
RUN cd launcher && GO111MODULE=on go run cmd/make/make.go -targets=launcher -linkstamp $FAKE

# Install
RUN mkdir -p /usr/local/kolide/bin/
RUN cp launcher/build/linux.*/* /usr/local/kolide/bin/
RUN cd launcher && GO111MODULE=on go run ./tools/download-osquery.go  --platform linux --output /usr/local/kolide/bin/osqueryd

# Set entrypoint
ENTRYPOINT ["/usr/local/kolide/bin/launcher"]
CMD []

# Don't need more than the artifacts for future things
FROM scratch
COPY --from=golauncherbuild /usr/local/kolide/bin/* /usr/local/kolide/bin/
