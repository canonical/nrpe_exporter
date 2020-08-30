FROM ubuntu:16.04 as build

ENV GOPATH=/go
WORKDIR /go/src/app
RUN apt-get update && apt-get install -y openssl libssl-dev curl git build-essential pkg-config golang

RUN go get golang.org/dl/go1.15
RUN /go/bin/go1.15 download
RUN ln -sf /go/bin/go1.15 /usr/bin/go

COPY . .

RUN make buildstatic

FROM alpine:latest

COPY --from=build /go/src/app/nrpe_exporter /bin/nrpe_exporter

EXPOSE      9275
ENTRYPOINT  [ "/bin/nrpe_exporter" ]
