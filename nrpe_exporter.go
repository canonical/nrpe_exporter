package main

import (
	"net"
	"net/http"
	"os"
	"time"

	"github.com/aperum/nrpe"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/spacemonkeygo/openssl"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9275").String()
)

// Collector type containing issued command and a logger
type Collector struct {
	command string
	args    string
	target  string
	ssl     bool
	logger  log.Logger
}

// CommandResult type describing the result of command against nrpe-server
type CommandResult struct {
	commandDuration float64
	statusOk        float64
	result          *nrpe.CommandResult
}

// Describe implemented with dummy data to satisfy interface
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("dummy", "dummy", nil, nil)
}

func collectCommandMetrics(cmd string, args string, conn net.Conn, logger log.Logger) (CommandResult, error) {
	// Parse and issue given command
	command := nrpe.NewCommand(cmd, args)
	startTime := time.Now()
	result, err := nrpe.Run(conn, command, false, 0)
	if err != nil {
		return CommandResult{}, err
	}
	duration := time.Since(startTime).Seconds()
	level.Debug(logger).Log("msg", "Command returned", "command", command.Name, "duration", duration, "result", result.StatusLine)
	statusOk := 1.0
	if result.StatusCode != 0 {
		statusOk = 0
		level.Debug(logger).Log("msg", "Status code did not equal 0", "status code", result.StatusCode)
	}
	return CommandResult{duration, statusOk, result}, nil
}

// Collect dials nrpe-server and issues given command, recording metrics based on the result.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var ctx *openssl.Ctx
	var conn net.Conn
	var err error
	// Connect to NRPE server
	if c.ssl {
		ctx, err = openssl.NewCtx()
		if err != nil {
			level.Error(c.logger).Log("msg", "Error creating SSL context", "err", err)
			return
		}
		err = ctx.SetCipherList("ALL:!MD5:@STRENGTH")
		if err != nil {
			level.Error(c.logger).Log("msg", "Error setting SSL cipher list", "err", err)
			return
		}
		conn, err = openssl.Dial("tcp", c.target, ctx, openssl.InsecureSkipHostVerification)
	} else {
		conn, err = net.Dial("tcp", c.target)
	}
	if err != nil {
		level.Error(c.logger).Log("msg", "Error dialing NRPE server", "err", err)
		return
	}
	defer conn.Close()

	cmdResult, err := collectCommandMetrics(c.command, c.args, conn, c.logger)
	if err != nil {
		level.Error(c.logger).Log("msg", "Error running command", "command", c.command, "err", err)
		return
	}

	// Create metrics based on results of given command
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("command_duration", "Length of time the NRPE command took", nil, nil),
		prometheus.GaugeValue,
		cmdResult.commandDuration,
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("command_ok", "Indicates whether or not the command was a success", nil, nil),
		prometheus.GaugeValue,
		cmdResult.statusOk,
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("command_status", "Indicates the status of the command", nil, nil),
		prometheus.GaugeValue,
		float64(cmdResult.result.StatusCode),
	)
}

// NewCollector returns new collector with logger and given command
func NewCollector(command, args, target string, ssl bool, logger log.Logger) *Collector {
	return &Collector{
		command: command,
		args:    args,
		target:  target,
		ssl:     ssl,
		logger:  logger,
	}
}

func handler(w http.ResponseWriter, r *http.Request, logger log.Logger) {
	params := r.URL.Query()
	target := params.Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", 400)
		return
	}
	cmd := params.Get("command")
	if cmd == "" {
		http.Error(w, "Command parameter is missing", 400)
		return
	}
	args := params.Get("args")
	sslParam := params.Get("ssl")
	ssl := sslParam == "true"
	registry := prometheus.NewRegistry()
	collector := NewCollector(cmd, args, target, ssl, logger)
	registry.MustRegister(collector)
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

func main() {
	logConfig := promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, &logConfig)
	kingpin.Version(version.Print("nrpe_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(&logConfig)
	level.Info(logger).Log("msg", "Starting nrpe_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
            <head>
            <title>NRPE Exporter</title>
            </head>
            <body>
            <h1>NRPE Exporter</h1>
						<p><a href="/metrics">Metrics</a></p>
						<p><a href="/export?command=check_load&target=127.0.0.1:5666">check_load against localhost:5666</a></p>
            </body>
            </html>`))
	})

	http.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, logger)
	})
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server")
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
}
