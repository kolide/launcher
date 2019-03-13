ARG FAKE
FROM launcher${FAKE}-build as stage1

FROM gcr.io/distroless/base
LABEL maintainer="engineering@kolide.co"

# RUN mkdir -p /usr/local/kolide/bin/
COPY --from=stage1 /usr/local/kolide/bin/* /usr/local/kolide/bin/

# Set entrypoint
ENTRYPOINT ["/usr/local/kolide/bin/launcher"]
CMD []
