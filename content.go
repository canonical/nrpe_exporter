package main

import (
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"runtime/debug"

	"github.com/prometheus/common/version"
)

const (
	docsUrl   = "https://github.com/peekjef72/nrpe_exporter#readme"
	templates = `
    {{ define "page" -}}
      <html>
      <head>
        <title>Prometheus {{ .ExporterName }}</title>
        <style type="text/css">
          body { margin: 0; font-family: "Helvetica Neue", Helvetica, Arial, sans-serif; font-size: 14px; line-height: 1.42857143; color: #333; background-color: #fff; }
          .navbar { display: flex; background-color: #222; margin: 0; border-width: 0 0 1px; border-style: solid; border-color: #080808; }
          .navbar > * { margin: 0; padding: 15px; }
          .navbar * { line-height: 20px; color: #9d9d9d; }
          .navbar a { text-decoration: none; }
          .navbar a:hover, .navbar a:focus { color: #fff; }
          .navbar-header { font-size: 18px; }
          body > * { margin: 15px; padding: 0; }
          pre { padding: 10px; font-size: 13px; background-color: #f5f5f5; border: 1px solid #ccc; }
          h1, h2 { font-weight: 500; }
          a { color: #337ab7; }
          a:hover, a:focus { color: #23527c; }
		  table { border: 1px solid #edd2e6; border-collapse: collapse; margin-bottom: 1rem; width: 80%; }
		  tr { border: 1px solid #edd2e6; padding: 0.3rem; text-align: left; width: 35%; }
		  th { border: 1px solid #edd2e6; padding: 0.3rem; }
		  td { border: 1px solid #edd2e6; padding: 0.3rem; }
		  .odd { background-color: rgba(0,0,0,.05); }
        </style>
      </head>
      <body>
        <div class="navbar">
          <div class="navbar-header"><a href="/">Prometheus {{ .ExporterName }}</a></div>
          <div><a href="/healthz">Health</a></div>
		  <div><a href="{{ .ExportPath }}?command=check_load&target=127.0.0.1:5666&metric_name=nrpe_load">Export</a></div>
          <div><a href="{{ .ProfilePath }}">Profiles</a></div>
          <div><a href="/status">Status</a></div>
          <div><a href="{{ .MetricsPath }}">Exporter Metrics</a></div>
          <div><a href="{{ .DocsUrl }}">Help</a></div>
        </div>
        {{template "content" .}}
      </body>
      </html>
    {{- end }}

    {{ define "content.home" -}}
      <h1>This is a <a href="{{ .DocsUrl }}">Prometheus {{ .ExporterName }}</a> instance.</h1>
        <p>You are probably looking for its metrics:</p>
			<p><strong>E.G.:</strong></p>
			<li>nrpe without ssl and no parameter local check_load: <a href="{{ .ExportPath }}?command=check_load&target=127.0.0.1:5666&metric_name=nrpe_load">check_load against localhost:5666 NO SSL</a>.</li>
			<li>nrpe with ssl and no parameter local check_load: <a href="{{ .ExportPath }}?ssl=true&command=check_load&target=127.0.0.1:5666&metric_name=nrpe_load&result_message=true&performance=true">check_load against localhost:5666 SSL</a>.</li>
			<li>nrpe with ssl and allow parameters local check_load: <a href="{{ .ExportPath }}?ssl=true&command=check_load&params=params=-r%20-w%20.15,.10,.05%20-c%20.30,.25,.20&target=127.0.0.1:5666&metric_name=nrpe_load&result_message=true&performance=true">check_load against localhost:5666 SSL ALLOW param</a>.</li>
    {{- end }}

    {{ define "content.profiles" -}}
      <h2>Profiles</h2>
      <pre>{{ .Profiles }}</pre>
    {{- end }}

	{{ define "content.status" -}}
	<h2>Build Information</h2>
	<table>
	  	<tbody>
			<tr class="odd" >
				<th>Version</th>
				<td>{{ .Version.Version }}</td>
			</tr>
			<tr>
				<th>Revision</th>
				<td>{{ .Version.Revision }}</td>
			</tr>
			<tr class="odd" >
				<th>Branch</th>
				<td>{{ .Version.Branch }}</td>
			</tr>
			<tr>
				<th>BuildUser</th>
				<td>{{ .Version.BuildUser }}</td>
			</tr>
			<tr class="odd" >
				<th>BuildDate</th>
				<td>{{ .Version.BuildDate }}</td>
			</tr>
			<tr>
				<th>BuildTags</th>
				<td>{{ .Version.BuildTags }}</td>
			</tr>
			<tr class="odd" >
				<th>GoVersion</th>
				<td>{{ .Version.GoVersion }}</td>
			</tr>
		</tbody>
	</table>
  {{- end }}

    {{ define "content.error" -}}
      <h2>Error</h2>
      <pre>{{ .Err }}</pre>
    {{- end }}
    `
)

type versionInfo struct {
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	BuildTags string
	GoVersion string
}
type tdata struct {
	ExporterName string
	ExportPath   string
	MetricsPath  string
	ProfilePath  string
	DocsUrl      string

	// `/profiles` only
	Profiles string

	// status
	Version versionInfo
	// `/error` only
	Err error
}

var (
	allTemplates    = template.Must(template.New("").Parse(templates))
	homeTemplate    = pageTemplate("home")
	profileTemplate = pageTemplate("profiles")
	statusTemplate  = pageTemplate("status")
	errorTemplate   = pageTemplate("error")
)

func pageTemplate(name string) *template.Template {
	pageTemplate := fmt.Sprintf(`{{define "content"}}{{template "content.%s" .}}{{end}}{{template "page" .}}`, name)
	return template.Must(template.Must(allTemplates.Clone()).Parse(pageTemplate))
}

// HomeHandlerFunc is the HTTP handler for the home page (`/`).
func HomeHandlerFunc(exporter Exporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		homeTemplate.Execute(w, &tdata{
			ExporterName: exporter.ExporterName,
			ExportPath:   exporter.ExporterPath,
			MetricsPath:  exporter.MetricPath,
			ProfilePath:  exporter.ProfilesPath,
			DocsUrl:      docsUrl,
		})
	}
}

// ConfigHandlerFunc is the HTTP handler for the `/config` page. It outputs the configuration marshaled in YAML format.
func ProfilesHandlerFunc(exporter Exporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		profiles, err := exporter.Profiles.Dump()
		if err != nil {
			HandleError(0, err, exporter, w, r)
			return
		}
		profileTemplate.Execute(w, &tdata{
			ExporterName: exporter.ExporterName,
			ExportPath:   exporter.ExporterPath,
			MetricsPath:  exporter.MetricPath,
			ProfilePath:  exporter.ProfilesPath,
			DocsUrl:      docsUrl,
			Profiles:     profiles,
		})
	}
}

// ConfigHandlerFunc is the HTTP handler for the `/config` page. It outputs the configuration marshaled in YAML format.
func StatusHandlerFunc(exporter Exporter) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		vinfos := versionInfo{
			Version:   version.Version,
			Revision:  version.Revision,
			Branch:    version.Branch,
			BuildUser: version.BuildUser,
			BuildDate: version.BuildDate,
			BuildTags: computeTags(),
			GoVersion: runtime.Version(),
		}

		statusTemplate.Execute(w, &tdata{
			ExporterName: exporter.ExporterName,
			ExportPath:   exporter.ExporterPath,
			MetricsPath:  exporter.MetricPath,
			ProfilePath:  exporter.ProfilesPath,
			DocsUrl:      docsUrl,
			Version:      vinfos,
		})
	}
}

// HandleError is an error handler that other handlers defer to in case of error. It is important to not have written
// anything to w before calling HandleError(), or the 500 status code won't be set (and the content might be mixed up).
func HandleError(status int, err error, exporter Exporter, w http.ResponseWriter, r *http.Request) {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	w.WriteHeader(status)
	errorTemplate.Execute(w, &tdata{
		ExporterName: exporter.ExporterName,
		ExportPath:   exporter.ExporterPath,
		MetricsPath:  exporter.MetricPath,
		ProfilePath:  exporter.ProfilesPath,
		DocsUrl:      docsUrl,
		Err:          err,
	})
}

func computeTags() string {
	var (
		tags = "unknown"
	)

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return tags
	}
	for _, v := range buildInfo.Settings {
		if v.Key == "-tags" {
			tags = v.Value
		}
	}
	return tags
}
