FROM alpine:latest

RUN apk update && apk add --no-cache openssl

RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

COPY nrpe_exporter  /bin/nrpe_exporter

EXPOSE      9275
ENTRYPOINT  [ "/bin/nrpe_exporter" ]
