#!/bin/sh

version="$(git rev-parse --short HEAD)"
tar -czf latest.tar.gz build/darwin build/linux
gsutil cp latest.tar.gz "gs://kolide-build-cache/launcher/latest.tar.gz"
gsutil cp latest.tar.gz "gs://kolide-build-cache/launcher/${version}.tar.gz"
