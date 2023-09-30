# NRPE exporter

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
  - job_name: nrpe_check_load
    metrics_path: /export
    params:
      command: [check_load] # Run the check_load command.
      ssl: [true] # if using ssl
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

Result Codes in command_status:
```
    StatusOK       = 0
    StatusWarning  = 1
    StatusCritical = 2
    StatusUnknown  = 3

```
Sample Alert Rule:
```

groups:
- name: NRPE Host Load Status
  rules:
  - alert: HighLoad
    expr: avg_over_time(command_status{job="nrpe_check_load"}[5m]) > 0
    for: 5m
    annotations:
      summary: "Load is high {{ $labels.instance }}"
      description: "{{ $labels.instance }} for job {{ $labels.job }} has sustained high load."

```

## command arg support
### command:
* command:
* params:

curl -v "http://localhost:9275/export?command=check_disk&params=-Xbinfmt_misc%20-X%20devpts%20-X%20devtmpfs%20-X%20noner%20-X%20proc%20-X%20procfs%20-X%20rpc_pipefs%20-X%20sysfs%20-X%20tmpfs%20-X%20overlay%20-X%20debugfs%20-X%20tracefs%20-X%20autofs%20-X%20cgroup%20-X%20iso9660%20--errors-only%20-t10%20-w5%20-c3%20--all%20-i%2F.snapshot%2F%20-i%20podman%20-i%2Frun%20-u%20kB&target=127.0.0.1:5666&ssl=true

```
# HELP command_duration Length of time the NRPE command took in second
# TYPE command_duration gauge
command_duration{command="check_disk"} 0.003563082
# HELP command_ok Indicates whether or not the command was a success (0: cmd status code did not equal 0 | 1: ok)
# TYPE command_ok gauge
command_ok{command="check_disk"} 1
# HELP command_status Indicates the status of the command (nrpe status: 0: OK | 1: WARNING | 2: CRITICAL | 3: UNKNOWN)
# TYPE command_status gauge
command_status{command="check_disk"} 0
# HELP nrpe_scrap_duration Length of time the NRPE commands took
# TYPE nrpe_scrap_duration gauge
nrpe_scrap_duration 0.005256659
# HELP nrpe_up Indicates whether or not nrpe agent is ip
# TYPE nrpe_up gauge
nrpe_up 1
```


Profiles:

## Performance Data support


* curl -v "http://localhost:9275/export?command=check_load&target=127.0.0.1:5666&ssl=true"

```
# HELP check_load_load1 the NRPE command perfdata value
# TYPE check_load_load1 gauge
check_load_load1 0.17
# HELP check_load_load15 the NRPE command perfdata value
# TYPE check_load_load15 gauge
check_load_load15 0.16
# HELP check_load_load5 the NRPE command perfdata value
# TYPE check_load_load5 gauge
check_load_load5 0.18
```

* curl -v "http://localhost:9275/export?command=check_load&target=127.0.0.1:5666&ssl=true&metricname=nrpe_load"

```
# HELP nrpe_load the NRPE command perfdata value
# TYPE nrpe_load gauge
nrpe_load{check_load="load1"} 0.64
nrpe_load{check_load="load15"} 0.17
nrpe_load{check_load="load5"} 0.23
```


curl -v "http://localhost:9275/export?command=check_load&target=127.0.0.1:5666&ssl=true&metricname=nrpe_load&labelname=load_type

```
# HELP nrpe_load the NRPE command perfdata value
# TYPE nrpe_load gauge
nrpe_load{load_type="load1"} 0.06
nrpe_load{load_type="load15"} 0.15
nrpe_load{load_type="load5"} 0.12
```

* performance: yes no true false.
* metricname: comma separated list of metric name in the same order that the performance counter element
labelname: 

curl -v "http://localhost:9275/export?command=check_disk&params=-Xbinfmt_misc%20-X%20devpts%20-X%20devtmpfs%20-X%20noner%20-X%20proc%20-X%20procfs%20-X%20rpc_pipefs%20-X%20sysfs%20-X%20tmpfs%20-X%20overlay%20-X%20debugfs%20-X%20tracefs%20-X%20autofs%20-X%20cgroup%20-X%20iso9660%20--errors-only%20-t10%20-w5%20-c3%20--all%20-i%2F.snapshot%2F%20-i%20podman%20-i%2Frun%20-u%20kB&target=127.0.0.1:5666&ssl=true&metricname=filesystem_used_kilobytes,,,,filesystem_total_kilobytes&labelname=filesystem&performance=yes"


```
# HELP command_duration Length of time the NRPE command took in second
# TYPE command_duration gauge
command_duration{command="check_disk"} 0.003572237
# HELP command_ok Indicates whether or not the command was a success (0: cmd status code did not equal 0 | 1: ok)
# TYPE command_ok gauge
command_ok{command="check_disk"} 1
# HELP command_status Indicates the status of the command (nrpe status: 0: OK | 1: WARNING | 2: CRITICAL | 3: UNKNOWN)
# TYPE command_status gauge
command_status{command="check_disk"} 0
# HELP filesystem_total_kilobytes the NRPE command perfdata value
# TYPE filesystem_total_kilobytes gauge
filesystem_total_kilobytes{filesystem="/"} 2.47343104e+08
filesystem_total_kilobytes{filesystem="/boot"} 1.038336e+06
filesystem_total_kilobytes{filesystem="/home"} 2.66206212e+08
# HELP filesystem_used_kilobytes the NRPE command perfdata value
# TYPE filesystem_used_kilobytes gauge
filesystem_used_kilobytes{filesystem="/"} 1.1646244e+07
filesystem_used_kilobytes{filesystem="/boot"} 429308
filesystem_used_kilobytes{filesystem="/home"} 1.4749668e+07
# HELP nrpe_scrap_duration Length of time the NRPE commands took
# TYPE nrpe_scrap_duration gauge
nrpe_scrap_duration 0.005689446
# HELP nrpe_up Indicates whether or not nrpe agent is ip
# TYPE nrpe_up gauge
nrpe_up 1
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

A trivial `Makefile` is provided, which will perform this task in Docker.
