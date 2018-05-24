FROM quay.io/prometheus/busybox:latest

COPY nrpe_exporter  /bin/nrpe_exporter

EXPOSE      9275
ENTRYPOINT  [ "/bin/nrpe_exporter" ]
