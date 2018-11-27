FROM golang:1.11
MAINTAINER Kolide Developers <engineering@kolide.co>

# Get add-apt-repository
RUN apt-get update && apt-get install -y \
  software-properties-common \
  apt-transport-https \
  gnupg

# Add the apt repository for osquery packages
RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 1484120AC4E9F8A1A577AEEE97A80C63C9D8B80B \
  && add-apt-repository "deb [arch=amd64] https://pkg.osquery.io/deb deb main"

# Install osquery
RUN apt-get update \
  && apt-get install -y osquery

RUN rm -rf /var/lib/apt/lists/*
