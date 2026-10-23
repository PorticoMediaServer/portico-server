FROM debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad
LABEL portico="remote-sources-r40"
RUN apt-get update \
    && apt-get install -y --no-install-recommends samba \
    && rm -rf /var/lib/apt/lists/* \
    && smbd --version
COPY smb.conf /etc/samba/smb.conf
CMD ["smbd", "--foreground", "--no-process-group", "--debug-stdout", "--configfile=/etc/samba/smb.conf"]
