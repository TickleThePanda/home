FROM debian:trixie-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends bind9 bind9-utils bind9-dnsutils \
 && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh

EXPOSE 53/tcp 53/udp

# named runs as root inside the container so it can bind :53 and write the
# zone journals on the mounted volume without ownership juggling.
ENTRYPOINT ["/entrypoint.sh"]
