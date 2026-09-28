FROM alpine:3.23

RUN apk add --no-cache curl python3

COPY provision.sh /opt/provision.sh

ENTRYPOINT ["/bin/sh", "/opt/provision.sh"]
