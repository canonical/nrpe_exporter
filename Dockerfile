FROM golang as build

WORKDIR /go/src/app
RUN apt-get update && apt-get install -y openssl libssl-dev
COPY nrpe_exporter.go .
RUN go get -d -v ./...
RUN go build -o nrpe_exporter .

# Multi-stage-build
FROM alpine:latest

RUN apk update && apk add --no-cache openssl

RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2

COPY --from=build /go/src/app/nrpe_exporter /bin/nrpe_exporter

EXPOSE      9275
ENTRYPOINT  [ "/bin/nrpe_exporter" ]
