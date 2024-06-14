package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/canonical/nrpe_exporter/profiles"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	nrpe "github.com/peekjef72/nrped/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"github.com/spacemonkeygo/openssl"
)

// **************
const (
	// Constant values
	metricsPublishingPort = ":9275"
)

var (
	//	listenAddress         = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9275").String()
	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, metricsPublishingPort)

	metricsPath           = kingpin.Flag("web.telemetry-path", "Path under which to expose collector's internal metrics.").Default("/metrics").String()
	exportPath            = kingpin.Flag("web.export-path", "Path under which to expose targets' metrics.").Default("/export").String()
	profilesPath          = kingpin.Flag("web.profiles-path", "Path under which to expose profiles configuration.").Default("/profiles").String()
	profilesFilePath      = kingpin.Flag("extend.profiles-file", "Path to custom command checks to run.").Default("").String()
	nrpe_packet_version   = kingpin.Flag("nrpe_packet_version", "nrpe packet version to use v2,v3 or v4(default)").Short('p').Default("4").Int()
	default_metric_prefix = kingpin.Flag("metric_prefix", "metric prefix to prepend to each metric").Short('m').Default("nrpe").String()
)

type Exporter struct {
	ExporterName     string
	MetricPath       string
	ExporterPath     string
	ProfilesPath     string
	ProfilesFilePath string

	Profiles *profiles.Profiles
	logger   log.Logger
}

// Collector type containing issued command and a logger
type Collector struct {
	cmds []*profiles.CommandConfig

	target         string
	ssl            bool
	metric_prefix  string
	packet_version int
	logger         log.Logger
	labels_re      *regexp.Regexp
	one_label_re   *regexp.Regexp
}

// CommandResult type describing the result of command against nrpe-server
type CommandResult struct {
	commandDuration float64
	statusOk        float64
	result          nrpe.NrpePacket
}

// Describe implemented with dummy data to satisfy interface
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("dummy", "dummy", nil, nil)
}

func collectCommandMetrics(cmd *nrpe.NrpeCommand, conn net.Conn, packet_version int, logger log.Logger) (CommandResult, error) {

	startTime := time.Now()

	// build nrpe packet
	pkt_to_send, err := nrpe.MakeNrpePacket(cmd.ToCommandLine(), nrpe.QUERY_PACKET, packet_version)
	if err != nil {
		level.Error(logger).Log("msg", err)
		return CommandResult{
			commandDuration: time.Since(startTime).Seconds(),
			statusOk:        0,
			result:          nil,
		}, err
	}

	if err := pkt_to_send.PrepareToSend(nrpe.QUERY_PACKET); err != nil {
		level.Error(logger).Log("msg", err)
		return CommandResult{
			commandDuration: time.Since(startTime).Seconds(),
			statusOk:        0,
			result:          nil,
		}, err
	}

	if err := pkt_to_send.SendPacket(conn); err != nil {
		level.Error(logger).Log("msg", err)
		return CommandResult{
			commandDuration: time.Since(startTime).Seconds(),
			statusOk:        0,
			result:          nil,
		}, err
	}

	result, err := nrpe.ReceivePacket(conn)
	if err != nil {
		level.Error(logger).Log("msg", err)
		return CommandResult{
			commandDuration: time.Since(startTime).Seconds(),
			statusOk:        0,
			result:          nil,
		}, err
	}

	duration := time.Since(startTime).Seconds()
	if ipaddr, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
		level.Info(logger).Log("msg", "Command returned", "command", cmd.Name,
			"address", ipaddr, "duration", duration, "return_code", result.ResultCode(),
			"command_output", result.GetCommandBuffer())
	}
	statusOk := 1.0
	if result.ResultCode() != 0 {
		statusOk = 0
	}
	return CommandResult{duration, statusOk, result}, nil
}

func getValue(val string, logger log.Logger) float64 {
	var val_num float64 = -1
	re := regexp.MustCompile(`^-?[0-9.]+`)
	value := re.FindString(val)
	if value != "" {
		val_num2, err := strconv.ParseFloat(value, 64)
		if err != nil {
			level.Error(logger).Log("invalid float value:", value)
		} else {
			val_num = val_num2
		}
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

// check the label name, if invalid char are found are replace them by '_'
func validateLabelValue(name string) string {
	if len(name) == 0 {
		name = "undefined_label"
	}
	return strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII {
			r = '_'
			return r
		} else {
			return r
		}
	}, name)
}

// Collect dials nrpe-server and issues given command, recording metrics based on the result.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var ctx *openssl.Ctx
	var conn net.Conn
	var err error

	// to compute profile scrap duration; if only one command same than command duration.
	startTime := time.Now()

	// Connect to NRPE server
	if c.ssl {
		ctx, err = openssl.NewCtx()
		if err == nil {
			err = ctx.SetCipherList("ALL:!MD5:@STRENGTH:@SECLEVEL=0")
			if err != nil {
				level.Error(c.logger).Log("msg", "Error setting SSL cipher list", "err", err)
			}
		} else {
			level.Error(c.logger).Log("msg", "Error creating SSL context", "err", err)
		}

		if err != nil {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc("nrpe_up", "Indicates whether or not nrpe agent is up", nil, nil),
				prometheus.GaugeValue,
				0,
			)
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
		if conn == (net.Conn)(nil) || err != nil {
			level.Error(c.logger).Log("msg", "Error dialing NRPE server", "err", err)
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc("nrpe_up", "Indicates whether or not nrpe agent is up", nil, nil),
				prometheus.GaugeValue,
				0,
			)
			return
		}
		defer conn.Close()

		cmdResult, err := collectCommandMetrics(cmd.Cmd, conn, c.packet_version, c.logger)
		if err != nil {
			level.Error(c.logger).Log("msg", "Error running command", "command", cmd.Command, "err", err)
		}

		// Create metrics based on results of given command
		labels := make([]string, 1)
		labels[0] = "command"

		// metric for status of the command 1: run - 0 not run
		name := "command_ok"
		//*** if metric_name defined by user it has to be a prefix for metric
		if c.metric_prefix != "" {
			name = strings.Join([]string{c.metric_prefix, name}, "_")
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(name, "Indicates whether or not the command was a success (0: cmd status code did not equal 0 | 1: ok)", labels, nil),
			prometheus.GaugeValue,
			float64(cmdResult.statusOk),
			cmd.Command,
		)
		if err != nil {
			continue
		}

		// metric for command result
		name = "command_status"

		//*** if metric_name defined by user it has to be a prefix for metric
		if c.metric_prefix != "" {
			name = strings.Join([]string{c.metric_prefix, name}, "_")
		}

		label_keys := make([]string, 1)
		label_values := make([]string, 1)
		label_keys[0] = "command"
		label_values[0] = cmd.Command
		if cmd.ResultMsg {
			loc_label_name := "command_result_msg"
			if c.metric_prefix != "" {
				loc_label_name = strings.Join([]string{c.metric_prefix, loc_label_name}, "_")
			}
			label_keys = append(label_keys, loc_label_name)
			// replace " by ' and , by ; => required for labels parsing name=value delimited by "" and ,
			label_values = append(label_values, strings.ReplaceAll(strings.ReplaceAll(cmdResult.result.GetCommandBuffer(), `,`, `;`), `"`, "'"))
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(name, "Indicates the status of the command (nrpe status: 0: OK | 1: WARNING | 2: CRITICAL | 3: UNKNOWN)", label_keys, nil),
			prometheus.GaugeValue,
			float64(cmdResult.result.ResultCode()),
			label_values...,
		)

		// metric for command duration
		name = "command_duration"
		//*** if metric_name defined by user it has to be a prefix for metric
		if c.metric_prefix != "" {
			name = strings.Join([]string{c.metric_prefix, name}, "_")
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(name, "Length of time the NRPE command took in second", labels, nil),
			prometheus.GaugeValue,
			cmdResult.commandDuration,
			cmd.Command,
		)

		// Parse Performance Data if present in result.CommandBuffer
		if cmd.PerfData {
			cmdBuffer := cmdResult.result.GetCommandBuffer()
			pos := strings.Index(cmdBuffer, "|")
			if pos > 0 {
				// will store the list of the metric names obtained from command configuration and that is a string in comma separated list of names
				var metric_names []string
				has_met_name := false

				if cmd.MetricName != "" {
					metric_names = strings.Split(cmd.MetricName, ",")
					has_met_name = true
				} else if cmd.MetricPrefix != "" {
					level.Debug(c.logger).Log("msg", "query has metric prefix", "raw names", cmd.MetricPrefix)
				}

				perfData := strings.Trim(cmdBuffer[pos+1:], " \n\r")
				level.Debug(c.logger).Log("msg", "have perfData", "raw_value", perfData)

				// set type and help for metric/value (default value set in constructor)
				metric_type := cmd.ValueType()
				help := cmd.Help
				if help == "" {
					help = "the NRPE command perfdata value"
				}

				// analyse received perfdata
				// format is : perfdata elment blank separated: [perfdata elmt1] [perfdata elmtX]
				params := strings.Split(perfData, " ")
				for _, param := range params {
					// perfdata element: format name=val1[;valx[;..]]
					pos = strings.Index(param, "=")
					if pos > -1 {
						// perf name can generate a metric name or a label_value depending on the context
						perf_name := param[0:pos]
						//*** if metric_name defined by user it has to be a prefix for metric
						if cmd.MetricPrefix != "" {
							perf_name = strings.Join([]string{cmd.MetricPrefix, perf_name}, "_")
						}
						perf_name = validateLabelValue(perf_name)

						level.Debug(c.logger).Log("msg", "found perfData", "metric_name", perf_name)
						raw_values := param[pos+1:]
						level.Debug(c.logger).Log("msg", "found perfData raw_value", "raw_value", raw_values)

						//** check value format: may be label(b64_string) or semicolumn separated list of numeric values
						values := c.labels_re.FindStringSubmatch(raw_values)
						if len(values) > 1 {
							//level.Debug(c.logger).Log("msg", "labels() function found.", "value", values[1])
							//* value is in base64
							raw_labels, err := base64.StdEncoding.DecodeString(values[1])
							if err != nil {
								level.Error(c.logger).Log("invalid b64 string in label", err, "received value", values[1])
								continue
							}
							labels_str := string(raw_labels)
							labels_str = strings.TrimFunc(labels_str, func(r rune) bool {
								return !unicode.IsGraphic(r)
							})

							level.Debug(c.logger).Log("msg", "labels function found.", "value", string(labels_str))
							//* split decoded lables string on blank
							labels := strings.Split(string(labels_str), " ")
							//level.Debug(c.logger).Log("msg", "labels .", "len", len(labels))
							//* init empty slice to store label_name
							label_names := make([]string, 0)
							label_values := make([]string, 0)
							for _, label := range labels {
								label_pairs := c.one_label_re.FindStringSubmatch(label)
								//							level.Debug(c.logger).Log("msg", "label_pairs() found ?", "res len", len(label_pairs))
								if len(label_pairs) < 3 {
									level.Info(c.logger).Log("msg", "invalid label_pair format regex failed: must be key=\"value\"")
									break
								}
								label_names = append(label_names, label_pairs[1])
								label_values = append(label_values, label_pairs[2])
								level.Debug(c.logger).Log("msg", "adding label_pairs", "label_name", label_pairs[1], "value", label_pairs[2])

							}
							value := 1.0
							if len(values) > 2 {
								value = getValue(values[2], c.logger)
							}
							if len(label_names) > 0 {
								level.Debug(c.logger).Log("msg", "will add metric", "name", perf_name, "value", value)
								// Create metrics based on perfdata name and value
								ch <- prometheus.MustNewConstMetric(
									prometheus.NewDesc(perf_name, help, label_names, nil),
									metric_type,
									value,
									label_values...,
								)
							}
							continue
						}
						//* presume format is val1[;val2[;...]]
						values = strings.Split(raw_values, ";")

						// user doesn't specify a metric name: each perf element generates a metric
						// metric name will be by order of priority:
						// a) if cmd metric prefix is set: [cmd metric prefix]_[perf_name]
						// b) is default metric prefix is set: [default metric prefix]_[perf_name]
						// c) else: [cmd Name]_[perf_name]
						if !has_met_name {
							value := getValue(values[0], c.logger)
							metric_name := perf_name
							if cmd.MetricPrefix != "" {
								// metric_name = validateMetricName(strings.Join([]string{cmd.MetricPrefix, perf_name}, "_"))
								metric_name = validateMetricName(perf_name)
								ch <- prometheus.MustNewConstMetric(
									prometheus.NewDesc(metric_name, help, nil, nil),
									metric_type,
									value,
								)
							} else {
								metric_name = validateMetricName(strings.ToLower(cmd.Command))
								labels := make([]string, 1)
								labels[0] = "label1"
								ch <- prometheus.MustNewConstMetric(
									prometheus.NewDesc(metric_name, help, labels, nil),
									metric_type,
									value,
									perf_name,
								)
							}

							level.Debug(c.logger).Log("msg", "will add metric", "metric_name", metric_name, "value", value)
							// Create metrics based on perfdata name and value
						} else {
							// user has specified a metric name: use it as is
							// metric will by default have a label to distinguish each different perf element.
							// label_name is by order of priority:
							// a) cmd label is it is set.
							// b) cmd name else
							// if label_name is set to value NONE: metric is generated without label !
							//	in this case user is responsible of duplicates if command generates several perf elements.
							// perf_name is used as a label_value

							// loop on each value
							for i, raw_value := range values {
								// check if we have a metric name for that value; if not skip it
								if len(metric_names) > i && metric_names[i] != "" {

									metric_name := validateMetricName(metric_names[i])
									value := getValue(raw_value, c.logger)

									labels := make([]string, 1)
									if cmd.LabelName != "" {
										if cmd.LabelName == "NONE" {
											level.Debug(c.logger).Log("msg", "will add metric", "metric_name", metric_name, "value:", value)
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
									level.Debug(c.logger).Log("msg", "will add metric", "metric_name", metric_name, "label_name", labels[0], "label_value", perf_name, "value:", value)
									// Create metrics based on perfdata name and value
									ch <- prometheus.MustNewConstMetric(
										prometheus.NewDesc(metric_name, help, labels, nil),
										metric_type,
										value,
										perf_name,
									)
								}
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
func NewCollector(target string, ssl bool, cmds []*profiles.CommandConfig, metric_prefix string, packet_version int, logger log.Logger) *Collector {
	return &Collector{
		target:         target,
		ssl:            ssl,
		cmds:           cmds,
		metric_prefix:  metric_prefix,
		packet_version: packet_version,
		logger:         logger,
		labels_re:      regexp.MustCompile(`^labels\(([^\)]+)\)(?:,([^ ]+))?`),
		one_label_re:   regexp.MustCompile(`^\s*([a-zA-Z0-9_]+)="([^"]+)"\s*`),
	}
}

func NewCommand(command string, cmd_params string, metricPrefix, metricName string, labelName string) *profiles.CommandConfig {
	tmp := nrpe.NewNrpeCommand(command, cmd_params)
	return &profiles.CommandConfig{
		Command:      command,
		Params:       cmd_params,
		MetricPrefix: metricPrefix,
		MetricName:   metricName,
		LabelName:    labelName,
		Cmd:          &tmp,
	}
}

// ***********************************************************************************************
func handler(w http.ResponseWriter, r *http.Request, exporter Exporter) {
	//	var cmd_params []string
	var (
		cmds          []*profiles.CommandConfig
		metric_prefix string
	)
	packet_version := *nrpe_packet_version

	params := r.URL.Query()
	target := params.Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", 400)
		return
	}

	profile_name := params.Get("profile")
	if profile_name != "" {
		if exporter.Profiles == nil {
			http.Error(w, "no profile defined!", 400)
			return
		}
		p, err := exporter.Profiles.FindProfileName(profile_name)
		if err != nil {
			http.Error(w, fmt.Sprintf("%v", err), 400)
			return
		}
		cmds = p.Commands
		packet_version = p.PacketVersion
		metric_prefix = p.MetricPrefix
	} else {
		cmd_name := params.Get("command")
		if cmd_name == "" {
			http.Error(w, "Command parameter is missing", 400)
			return
		}
		cmd_params_str := params.Get("params")

		// profile metric_prefix
		metric_prefix = *default_metric_prefix

		metricPrefix := params.Get("metricprefix")
		metricName := params.Get("metricname")
		labelName := params.Get("labelname")
		performance := params.Get("performance")
		cmd_result_str := params.Get("result_message")

		cmd := NewCommand(cmd_name, cmd_params_str, metricPrefix, metricName, labelName)
		cmd.PerfData = profiles.ToBoolean(performance)
		cmd.ResultMsg = profiles.ToBoolean(cmd_result_str)

		cmds = make([]*profiles.CommandConfig, 1)
		cmds[0] = cmd
	}
	sslParam := params.Get("ssl")
	ssl := sslParam == "true"

	registry := prometheus.NewRegistry()
	if metric_prefix == "" {
		metric_prefix = *default_metric_prefix
	}
	collector := NewCollector(target, ssl, cmds, metric_prefix, packet_version, exporter.logger)
	registry.MustRegister(collector)
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

type route struct {
	path    string
	handler http.HandlerFunc
}

func newRoute(path string, handler http.HandlerFunc) route {
	return route{path, handler}
}
func BuildHandler(exporter Exporter) http.Handler {
	var routes = []route{
		newRoute("/healthz", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) }),
		newRoute("/", HomeHandlerFunc(exporter)),
		newRoute("/status", StatusHandlerFunc(exporter)),
		newRoute(*profilesPath, ProfilesHandlerFunc(exporter)),
		newRoute(*exportPath, func(w http.ResponseWriter, r *http.Request) { handler(w, r, exporter) }),
		// Expose exporter metrics separately, for debugging purposes.
		newRoute(*metricsPath, func(w http.ResponseWriter, r *http.Request) { promhttp.Handler().ServeHTTP(w, r) }),
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, route := range routes {
			if req.URL.Path == route.path {
				route.handler(w, req)
				return
			}
		}
		err := fmt.Errorf("not found")
		HandleError(http.StatusNotFound, err, exporter, w, req)
	})
}

// ***********************************************************************************************
func main() {
	var profs *profiles.Profiles
	var err error

	logConfig := promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, &logConfig)
	kingpin.Version(version.Print("nrpe_exporter")).VersionFlag.Short('V')
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(&logConfig)
	level.Info(logger).Log("msg", "Starting nrpe_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	// read profile if specified
	if *profilesFilePath != "" {
		level.Info(logger).Log("msg", "reading profiles", "filepath", *profilesFilePath)
		profs, err = profiles.Load(*profilesFilePath)
		if err != nil {
			level.Error(logger).Log("Errmsg", "Error loading profiles", "err", err)
			os.Exit(1)
		}
		if profs != nil {
			level.Debug(logger).Log("msg", fmt.Sprintf("%d profile(s) found", len(profs.Profiles)))
		}
	}

	exporter := Exporter{
		ExporterName:     "NRPE Exporter",
		MetricPath:       *metricsPath,
		ExporterPath:     *exportPath,
		ProfilesPath:     *profilesPath,
		ProfilesFilePath: *profilesFilePath,
		Profiles:         profs,
		logger:           logger,
	}

	server := &http.Server{
		Handler: BuildHandler(exporter),
	}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
}
