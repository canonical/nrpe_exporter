#
# Prometheus nrpe_exporter docker file
# forked and heavily modified from https://github.com/RobustPerception/nrpe_exporter
# Ed Guy, August 2021
#

#FROM golang:alpine
FROM ubuntu:16.04
#ubuntu:16.04 needed for ciphers.

LABEL org.opencontainers.image.authors="edguy@eguy.org"

#RUN apk --no-cache add openssl git go pkgconfig openssl-dev musl-dev  libc-dev build-base
RUN apt update && apt install -y openssl git openssl libssl-dev musl-dev  libc-dev wget

RUN \
    wget https://dl.google.com/go/go1.16.4.linux-amd64.tar.gz \
    && tar -xvf go1.16.4.linux-amd64.tar.gz \
    && mv go /usr/local/

ENV GOROOT /usr/local/go
#GOPATH is the location of your work directory. For example my project directory is ~/Projects/Proj1 .
#ENV GOPATH /
#Now set the PATH variable to access go binary system wide.
ENV PATH $GOPATH/bin:$GOROOT/bin:$PATH

RUN which go
RUN printenv && go version && go env
RUN apt install -y gcc pkg-config lxc-dev

# not sure if this is needed - was found - NOT NEEDED for ubunti u version
#RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

WORKDIR /app
RUN git clone https://github.com/RobustPerception/nrpe_exporter.git . \
   && go build nrpe_exporter.go
#    && go build -a -ldflags '-extldflags "-static -ldl"' nrpe_exporter.go

EXPOSE      9275
ENTRYPOINT  [ "/app/nrpe_exporter", "--log.level=debug" ]
