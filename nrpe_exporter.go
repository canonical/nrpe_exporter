package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/canonical/nrpe_exporter/profiles"
	nrpe "github.com/canonical/nrped/common"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/spacemonkeygo/openssl"
)

var (
	listenAddress    = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9275").String()
	metricsPath      = kingpin.Flag("web.telemetry-path", "Path under which to expose collector's internal metrics.").Default("/metrics").String()
	exportPath       = kingpin.Flag("web.export-path", "Path under which to expose targets' metrics.").Default("/export").String()
	profilesPath     = kingpin.Flag("web.profiles-path", "Path to custom command checks to run.").Default("/profiles").String()
	profilesFilePath = kingpin.Flag("extend.profiles-file", "Path to custom command checks to run.").Default("").String()
)

// Collector type containing issued command and a logger
type Collector struct {
	cmds []*profiles.CommandConfig

	target string
	ssl    bool
	logger log.Logger
}

// CommandResult type describing the result of command against nrpe-server
type CommandResult struct {
	commandDuration float64
	statusOk        float64
	result          *nrpe.NrpePacket
}

// Describe implemented with dummy data to satisfy interface
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("dummy", "dummy", nil, nil)
}

func collectCommandMetrics(cmd string, conn net.Conn, logger log.Logger) (CommandResult, error) {
	// Parse and issue given command
	command := nrpe.PrepareToSend(cmd, nrpe.QUERY_PACKET)
	startTime := time.Now()
	err := nrpe.SendPacket(conn, command)
	if err != nil {
		return CommandResult{
			commandDuration: time.Since(startTime).Seconds(),
			statusOk:        0,
			result:          nil,
		}, err
	}

	result, err := nrpe.ReceivePacket(conn)
	if err != nil {
		level.Error(logger).Log("msg", "ERROR!", err)
		return CommandResult{
			commandDuration: time.Since(startTime).Seconds(),
			statusOk:        0,
			result:          nil,
		}, err
	}

	duration := time.Since(startTime).Seconds()
	if ipaddr, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err != nil {
		level.Info(logger).Log("msg", "Command returned", "command", cmd,
			"address", ipaddr, "duration", duration, "return_code", result.ResultCode,
			"command_output", string(bytes.Trim(result.CommandBuffer[:], "\x00")))
	}
	statusOk := 1.0
	if result.ResultCode != 0 {
		statusOk = 0
	}
	return CommandResult{duration, statusOk, &result}, nil
}

func getValue(val string, logger log.Logger) float64 {
	var val_num float64 = -1
	re := regexp.MustCompile(`^-?[0-9.]+`)
	value := re.FindString(val)
	val_num2, err := strconv.ParseFloat(value, 64)
	if err != nil {
		level.Error(logger).Log("invalid float value:", value)
	} else {
		val_num = val_num2
	}
	return val_num
}

func validateMetricName(name string) string {
	if len(name) == 0 {
		name = "undefined_metric_name"
	}
	new_name := []byte(name)
	for i, b := range new_name {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || b == ':' || (b >= '0' && b <= '9' && i > 0)) {
			new_name[i] = '_'
		}
	}
	return strings.ToLower(string(new_name))
}

// Collect dials nrpe-server and issues given command, recording metrics based on the result.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var ctx *openssl.Ctx
	var conn net.Conn
	var err error

	startTime := time.Now()

	// Connect to NRPE server
	if c.ssl {
		ctx, err = openssl.NewCtx()
		if err != nil {
			level.Error(c.logger).Log("msg", "Error creating SSL context", "err", err)
			return
		}
		err = ctx.SetCipherList("ALL:!MD5:@STRENGTH@SECLEVEL=0")
		if err != nil {
			level.Error(c.logger).Log("msg", "Error setting SSL cipher list", "err", err)
			return
		}
	}

	// loop on provided commands: need to open a cnx each time, because remote nrpe agent closes connection
	// after serving one command
	for _, cmd := range c.cmds {
		if c.ssl {
			conn, err = openssl.Dial("tcp", c.target, ctx, openssl.InsecureSkipHostVerification)
		} else {
			conn, err = net.Dial("tcp", c.target)
		}
		if err != nil {
			level.Error(c.logger).Log("msg", "Error dialing NRPE server", "err", err)
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc("nrpe_up", "Indicates whether or not nrpe agent is ip", nil, nil),
				prometheus.GaugeValue,
				0,
			)
			return
		}
		defer conn.Close()

		cmdResult, err := collectCommandMetrics(cmd.Command, conn, c.logger)
		if err != nil {
			level.Error(c.logger).Log("msg", "Error running command", "command", cmd.Command, "err", err)
		}

		labels := make([]string, 1)
		labels[0] = "command"
		// metric for status of the command 1: run - 0 not run
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("nrpe_command_ok", "Indicates whether or not the command was a success (0: cmd status code did not equal 0 | 1: ok)", labels, nil),
			prometheus.GaugeValue,
			float64(cmdResult.statusOk),
			cmd.Command,
		)
		if err != nil {
			continue
		}

		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("nrpe_command_status", "Indicates the status of the command (nrpe status: 0: OK | 1: WARNING | 2: CRITICAL | 3: UNKNOWN)", labels, nil),
			prometheus.GaugeValue,
			float64(cmdResult.result.ResultCode),
			cmd.Command,
		)

		// Create metrics based on results of given command
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("nrpe_command_duration", "Length of time the NRPE command took", nil, nil),
			prometheus.GaugeValue,
			cmdResult.commandDuration,
		)

		// Parse Performance Data if present in result.CommandBuffer
		cmdBuffer := string(bytes.Trim(cmdResult.result.CommandBuffer[:], "\x00"))
		pos := strings.Index(cmdBuffer, "|")
		if pos > 0 {
			var metric_names []string
			has_met_name := false

			if cmd.MetricName != "" {
				//level.Debug(c.logger).Log("msg", "query has metric names", "raw names", cmd.MetricName)
				metric_names = strings.Split(cmd.MetricName, ",")
				//level.Debug(c.logger).Log("msg", "query has metric names", "names", metric_names)
				has_met_name = true
				//level.Debug(c.logger).Log("msg", "query has metric names", "has_met_name", has_met_name)
			} // else {
			//	level.Debug(c.logger).Log("msg", "query has no metric names")
			//}

			//level.Debug(c.logger).Log("msg", "result has perfdata")
			perfData := strings.Trim(cmdBuffer[pos+1:], " \n\r")
			level.Debug(c.logger).Log("perfData", perfData)

			// set type and help for metric /value (default value set in constructor)
			//level.Debug(c.logger).Log("raw metric_type", cmd.TypeString)
			metric_type := cmd.ValueType()
			//level.Debug(c.logger).Log("metric_type", metric_type)
			help := cmd.Help
			if help == "" {
				help = "the NRPE command perfdata value"
				//level.Debug(c.logger).Log("set default value for help: ", help)
			}

			params := strings.Split(perfData, " ")
			for _, param := range params {
				pos = strings.Index(param, "=")
				if pos > -1 {
					name := param[0:pos]
					level.Debug(c.logger).Log("perfData var_name", name)
					raw_values := param[pos+1:]
					level.Debug(c.logger).Log("perfData raw_value", raw_values)
					values := strings.Split(raw_values, ";")
					if !has_met_name {
						value := getValue(values[0], c.logger)
						metric_name := validateMetricName(strings.Join([]string{cmd.Command, name}, "_"))

						level.Debug(c.logger).Log("will add metric:", metric_name, "value:", values)
						// Create metrics based on perfdata name and value
						ch <- prometheus.MustNewConstMetric(
							prometheus.NewDesc(metric_name, help, nil, nil),
							metric_type,
							value,
						)

					} else {
						// loop on each value
						for i, raw_value := range values {
							// check if we have a metric name for that value; if not skip it
							if len(metric_names) > i && metric_names[i] != "" {

								metric_name := validateMetricName(metric_names[i])
								value := getValue(raw_value, c.logger)

								labels := make([]string, 1)
								if cmd.LabelName != "" {
									if cmd.LabelName == "NONE" {
										level.Debug(c.logger).Log("will add metric:", metric_name, "value:", values)
										// Create metrics based on perfdata name and value
										ch <- prometheus.MustNewConstMetric(
											prometheus.NewDesc(metric_name, help, nil, nil),
											metric_type,
											value,
										)
										continue
									} else {
										labels[0] = strings.ToLower(cmd.LabelName)
									}
								} else {
									labels[0] = strings.ToLower(cmd.Command)
								}
								level.Debug(c.logger).Log("will add metric:", metric_name, "value:", values, "label", labels[0], "value:", name)
								// Create metrics based on perfdata name and value
								ch <- prometheus.MustNewConstMetric(
									prometheus.NewDesc(metric_name, help, labels, nil),
									metric_type,
									value,
									name,
								)

							}
						}
					}
				}
			}

		}

	}

	duration := time.Since(startTime).Seconds()
	// Create metrics based on results of given command
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("nrpe_scrap_duration", "Length of time the NRPE commands took", nil, nil),
		prometheus.GaugeValue,
		duration,
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("nrpe_up", "Indicates whether or not nrpe agent is ip", nil, nil),
		prometheus.GaugeValue,
		1,
	)

}

// NewCollector returns new collector with logger and given command
func NewCollector(target string, ssl bool, cmds []*profiles.CommandConfig, logger log.Logger) *Collector {
	return &Collector{
		target: target,
		ssl:    ssl,
		cmds:   cmds,
		logger: logger,
	}
}

func NewCommand(command string, cmd_params string, metricName string, labelName string) *profiles.CommandConfig {
	return &profiles.CommandConfig{
		Command:    command,
		Params:     cmd_params,
		MetricName: metricName,
		LabelName:  labelName,
	}
}

// ***********************************************************************************************
func handler(w http.ResponseWriter, r *http.Request, profs *profiles.Profiles, logger log.Logger) {
	//	var cmd_params []string
	var cmds []*profiles.CommandConfig

	params := r.URL.Query()
	target := params.Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", 400)
		return
	}

	profile_name := params.Get("profile")
	if profile_name != "" {
		if profs == nil {
			http.Error(w, "no profile defined!", 400)
			return
		}
		p, err := profs.FindProfileName(profile_name)
		if err != nil {
			http.Error(w, fmt.Sprintf("%v", err), 400)
			return
		}
		cmds = p.Commands
	} else {
		cmd_name := params.Get("command")
		if cmd_name == "" {
			http.Error(w, "Command parameter is missing", 400)
			return
		}
		cmd_params_str := params.Get("params")

		metricName := params.Get("metricname")
		labelName := params.Get("labelname")
		cmd := NewCommand(cmd_name, cmd_params_str, metricName, labelName)
		cmds = make([]*profiles.CommandConfig, 1)
		cmds[0] = cmd
	}
	sslParam := params.Get("ssl")
	ssl := sslParam == "true"

	registry := prometheus.NewRegistry()
	collector := NewCollector(target, ssl, cmds, logger)
	registry.MustRegister(collector)
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

// ***********************************************************************************************
func handlerProfile(w http.ResponseWriter, r *http.Request, profs *profiles.Profiles, logger log.Logger) {
	dump, err := profs.Dump()
	if err != nil {
		level.Error(logger).Log("Errmsg", "Error dumping profiles", "err", err)
	}
	var landingPage = []byte(`<html>
	    <head>
	    <title>NRPE Exporter</title>
	    </head>
	    <body>
	    <h1>NRPE Exporter Profiles</h1>
		<pre>` + dump + `
		</pre>
	    </body>
	    </html>
	`)
	//			<p><a href="` + *metricPath + `">Metrics</a></p>
	//			<p><a href="` + *exportPath + `"?command=check_load&target=127.0.0.1:5666">check_load against localhost:5666</a></p>
	//			<p><a href="` + *profilesPath + `">Profiles</a></p>
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)

	w.Header().Set("Content-Type", "text/html; charset=UTF-8") // nolint: errcheck
	w.Write(landingPage)                                       // nolint: errcheck
}

// ***********************************************************************************************
func main() {
	var profs *profiles.Profiles
	var err error

	logConfig := promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, &logConfig)
	kingpin.Version(version.Print("nrpe_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(&logConfig)
	level.Info(logger).Log("msg", "Starting nrpe_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	// read profile if specified
	if *profilesFilePath != "" {
		profs, err = profiles.Load(*profilesFilePath)
		if err != nil {
			level.Error(logger).Log("Errmsg", "Error loading profiles", "err", err)
			os.Exit(1)
		}

	}
	var landingPage = []byte(`<html>
            <head>
            <title>NRPE Exporter</title>
            </head>
            <body>
            <h1>NRPE Exporter</h1>
			<p><a href="` + *metricsPath + `">Metrics</a></p>
			<p><a href="` + *exportPath + `"?command=check_load&target=127.0.0.1:5666">check_load against localhost:5666</a></p>
			<p><a href="` + *profilesPath + `">Profiles</a></p>
            </body>
	    </html>
	`)
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(landingPage)
	})

	http.HandleFunc(*exportPath, func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, profs, logger)
	})

	http.HandleFunc(*profilesPath, func(w http.ResponseWriter, r *http.Request) {
		handlerProfile(w, r, profs, logger)
	})

	http.Handle(*metricsPath, promhttp.Handler())

	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server")
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
}
