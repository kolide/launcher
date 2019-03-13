ARG FAKE
FROM launcher${FAKE}-build as stage1

FROM centos:centos6
LABEL maintainer="engineering@kolide.co"

RUN mkdir -p /usr/local/kolide/bin/
COPY --from=stage1 /usr/local/kolide/bin/* /usr/local/kolide/bin/

# Launcher does cert verification. Ensure certs.
#RUN apt-get update && apt-get install ca-certificates -y

# Set entrypoint
ENTRYPOINT ["/usr/local/kolide/bin/launcher"]
CMD []
