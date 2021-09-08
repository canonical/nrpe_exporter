#
# Prometheus nrpe_exporter docker file
#

FROM ubuntu:16.04 as builder
#ubuntu:16.04 needed for ciphers.

ENV GOROOT /usr/local/go
ENV PATH $GOPATH/bin:$GOROOT/bin:$PATH
WORKDIR /app

RUN apt update \
    && apt install -y  wget openssl \
    && apt install -y  git libssl-dev musl-dev  libc-dev gcc pkg-config lxc-dev \
    && wget https://dl.google.com/go/go1.16.4.linux-amd64.tar.gz \
    && tar -xvf go1.16.4.linux-amd64.tar.gz \
    && mv go /usr/local/
COPY . .
RUN go build nrpe_exporter.go \
    && apt remove  -y  git libssl-dev musl-dev  libc-dev gcc pkg-config lxc-dev \
    && apt autoremove -y

FROM alpine:3.8
# latest (.14) does not have libssl.so.1.0.0

# Error loading shared library libssl.so.1.0.0: No such file or directory (needed by nrpe_exporter)
RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2 \
    && apk add --no-cache libssl1.0

COPY --from=builder /app/nrpe_exporter /bin/nrpe_exporter
EXPOSE      9275

ENTRYPOINT  [ "/bin/nrpe_exporter" ]
