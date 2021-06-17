# NRPE exporter [![Build Status](https://travis-ci.org/RobustPerception/nrpe_exporter.svg?branch=master)](https://travis-ci.org/RobustPerception/nrpe_exporter) [![CircleCI](https://circleci.com/gh/RobustPerception/nrpe_exporter.svg?style=shield)](https://circleci.com/gh/RobustPerception/nrpe_exporter)

The NRPE exporter exposes metrics on commands sent to a running NRPE daemon.

## Building and running

### Local Build

    go build nrpe_exporter.go
    ./nrpe_exporter

Visiting [http://localhost:9275/export?command=check_load&target=127.0.0.1:5666](http://localhost:9275/export?command=check_load&target=127.0.0.1:5666)
will return metrics for the command 'check_load' against a locally running NRPE server.

### Building with Docker

    docker build -t nrpe_exporter .
    docker run -d -p 9275:9275 --name nrpe_exporter

## Configuration

The nrpe_exporter requires little to no configuration.

The few options available such as logging level and the port to run on are configured via command line flags.

Run `./nrpe_exporter -h` to view all available flags.

## Prometheus Configuration

Example config:
```yml
global:
  scrape_interval: 10s
scrape_configs:
  - job_name: nrpe
    metrics_path: /export
    params:
      command: [check_load] # Run the check_load command.
    static_configs:
      - targets: # Targets to run the specified command against.
        - '127.0.0.1:5666'
        - 'example.com:5666'
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:9275 # Nrpe exporter.

```
Example config with arguments passed to command:
```yml
global:
  scrape_interval: 10s
scrape_configs:
  - job_name: nrpe
    metrics_path: /export
    params:
      command: [check_proc] # Run the check_proc command with arguments.
      args: [1:1!1:1!/opt/prometheus/alertmanager]
    static_configs:
      - targets: # Targets to run the specified command against.
        - '127.0.0.1:5666'
        - 'example.com:5666'
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [command]
        target_label: __param_command
      - source_labels: [args]
        target_label: __param_args
      - source_labels: [__param_target]
        target_label: instance
    - target_label: __address__
      replacement: 127.0.0.1:9275 # Nrpe exporter.
```


## SSL support

Add URL query parameter `ssl=true` to enable SSL for the NRPE connection, e.g.

```
    params:
      command: [check_load]
      ssl: [true]
```

NRPE requires the ADH ciphersuite which is not built by default in modern
version of openssl. If the following command returns nothing then you will
not be able to use it:

```
openssl ciphers -s -v ALL | grep ADH   # remove -s on older versions of openssl
```

The solution is to build a statically-linked `nrpe_exporter` binary on an
older server - Ubuntu 16.04 works.

```
go build -a -ldflags '-extldflags "-static -ldl"'
```

[A future version of golang](https://github.com/golang/go/issues/26492) may
provide a simpler way of doing this.
