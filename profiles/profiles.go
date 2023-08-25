package profiles

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	nrpe "github.com/peekjef72/nrped/common"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"
)

// Load attempts to parse the given config file and return a Config object.
func Load(profilesFile string) (*Profiles, error) {
	//	log.Infof("Loading profiles from %s", profilesFile)
	buf, err := os.ReadFile(profilesFile)
	if err != nil {
		return nil, err
	}

	p := Profiles{}
	err = yaml.Unmarshal(buf, &p)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// **********************************************
//
//	Profiles
//
// **********************************************
type Profiles struct {
	Profiles []*ProfileConfig `yaml:"profiles"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Config.
func (p *Profiles) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Profiles
	if err := unmarshal((*plain)(p)); err != nil {
		return err
	}

	if len(p.Profiles) == 0 {
		return fmt.Errorf("at least one profile must be defined")
	}

	return checkOverflow(p.XXX, "config")
}

func (p *Profiles) FindProfileName(name string) (*ProfileConfig, error) {
	var pc *ProfileConfig
	var err error
	if name == "" {
		err = errors.New("undefined name specified")
	}
	for _, pc = range p.Profiles {
		if pc.Name == name {
			break
		}
	}
	if pc == nil {
		err = errors.New(strings.Join([]string{"profile not found '", name, "'"}, ""))
	}
	return pc, err
}

func (p *Profiles) Dump() (string, error) {
	var str string
	dump, err := yaml.Marshal(p)
	if err == nil {
		str = string(dump)
	}

	return str, err
}

// **********************************************
//
//	Profile
//
// **********************************************
type ProfileConfig struct {
	Name             string             `yaml:"profile"`
	PerfData         ConvertibleBoolean `yaml:"performance"`
	PacketVersionStr string             `yaml:"packet_version,omitempty"`
	Commands         []*CommandConfig   `yaml:"commands"`

	PacketVersion int
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for ProfileConfig.
func (c *ProfileConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.PerfData = true
	type plain ProfileConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if len(c.Commands) == 0 {
		return fmt.Errorf("no commands defined for profile %q", c.Name)
	}

	// force all commands to not collect perfdata if profile is set to false.
	if !c.PerfData {
		for _, cmd := range c.Commands {
			cmd.PerfData = false
		}
	}

	if c.PacketVersionStr == "" {
		c.PacketVersion = nrpe.NRPE_PACKET_VERSION_4
	} else {
		if val, err := strconv.Atoi(c.PacketVersionStr); err == nil {
			if val >= nrpe.NRPE_PACKET_VERSION_1 && val <= nrpe.NRPE_PACKET_VERSION_4 {
				c.PacketVersion = val
			} else {
				c.PacketVersion = nrpe.NRPE_PACKET_VERSION_4
			}
		}
	}

	return checkOverflow(c.XXX, "profiles")
}

// **********************************************
//
//	Command
//
// **********************************************
type CommandConfig struct {
	Command      string             `yaml:"command"`
	Params       string             `yaml:"params,omitempty"`
	TypeString   string             `yaml:"type,omitempty"` // the Prometheus metric type
	Help         string             `yaml:"help,omitempty"` //the Prometheus metric help text
	MetricPrefix string             `yaml:"metric_prefix,omitempty"`
	MetricName   string             `yaml:"metric_name,omitempty"`
	LabelName    string             `yaml:"label_name,omitempty"`
	PerfData     ConvertibleBoolean `yaml:"performance,omitempty"`
	ResultMsg    ConvertibleBoolean `yaml:"result_message,omitempty"`

	valueType prometheus.ValueType // TypeString converted to prometheus.ValueType
	Cmd       *nrpe.NrpeCommand
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline" json:"-"`
}

// ValueType returns the metric type, converted to a prometheus.ValueType.
func (c *CommandConfig) ValueType() prometheus.ValueType {
	if c.TypeString == "" {
		c.valueType = prometheus.GaugeValue
	}
	return c.valueType
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for MetricConfig.
func (c *CommandConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.PerfData = true
	type plain CommandConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Check required fields
	if c.Command == "" {
		return fmt.Errorf("missing command for command %+v", c)
	}
	if c.TypeString != "" {
		switch strings.ToLower(c.TypeString) {
		case "counter":
			c.valueType = prometheus.CounterValue
		case "gauge":
			c.valueType = prometheus.GaugeValue
		default:
			return fmt.Errorf("unsupported metric type: %s", c.TypeString)
		}
	} else {
		c.valueType = prometheus.GaugeValue
	}
	// c.cmd
	tmp := nrpe.NewNrpeCommand(c.Command, c.Params)
	c.Cmd = &tmp

	return checkOverflow(c.XXX, "profiles")
}

// ConvertibleBoolean special type to retrive 1 yes true to boolean true
type ConvertibleBoolean bool

func (bit *ConvertibleBoolean) UnmarshalJSON(data []byte) error {
	asString := string(data)
	if asString == "1" || asString == "true" {
		*bit = true
	} else if asString == "0" || asString == "false" {
		*bit = false
	} else {
		return fmt.Errorf("boolean unmarshal error: invalid input %s", asString)
	}
	return nil
}

func ToBoolean(val string) ConvertibleBoolean {
	res := false
	if val == "1" || val == "true" || val == "yes" {
		res = true
	}
	return (ConvertibleBoolean)(res)
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}
