FROM kolide/ubuntu16-osquery
MAINTAINER engineering@kolide.co

COPY build/linux/ /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/launcher", "-osqueryd_path", "/usr/bin/osqueryd", "-debug"]