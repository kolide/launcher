ARG FAKE
FROM launcher${FAKE}-build as stage1

FROM centos:centos6
LABEL maintainer="engineering@kolide.co"

COPY --from=stage1 /usr/local/kolide/bin/* /usr/local/kolide/bin/

# Set entrypoint
ENTRYPOINT ["/usr/local/kolide/bin/launcher"]
CMD []
