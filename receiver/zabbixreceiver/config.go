package zabbixreceiver

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
)

var metricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

type Config struct {
	Mode      string          `mapstructure:"mode"`
	Streaming StreamingConfig `mapstructure:"streaming"`
	Schedule  ScheduleConfig  `mapstructure:"schedule"`
	Prom      PromConfig      `mapstructure:"prom"`
	Zabbix    ZabbixConfig    `mapstructure:"zabbix"`
	Metadata  MetadataConfig  `mapstructure:"metadata"`
}

type MetadataConfig struct {
	HostGroupsFormat  string   `mapstructure:"host_groups_format"`
	Enabled           bool     `mapstructure:"enabled"`
	InheritedHostTags bool     `mapstructure:"inherited_host_tags"`
	InventoryFields   []string `mapstructure:"inventory_fields"`
}

type StreamingConfig struct {
	EnrichWithAPI      bool                `mapstructure:"enrich_with_api"`
	Endpoint           string              `mapstructure:"endpoint"`
	Token              configopaque.String `mapstructure:"token"`
	MaxRequestBodySize int64               `mapstructure:"max_request_body_size"`
	Timeout            time.Duration       `mapstructure:"timeout"`
}

type ScheduleConfig struct {
	Jitter time.Duration `mapstructure:"jitter"`
	Jobs   JobsConfig    `mapstructure:"jobs"`
}

type JobsConfig struct {
	Discover JobConfig `mapstructure:"discover"`
	Values   JobConfig `mapstructure:"values"`
}

type JobConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	RunOnStart bool          `mapstructure:"run_on_start"`
	Interval   time.Duration `mapstructure:"interval"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

type PromConfig struct {
	Prefix      string            `mapstructure:"prefix"`
	ConstLabels map[string]string `mapstructure:"const_labels"`
}

type ZabbixConfig struct {
	URL     string              `mapstructure:"url"`
	Token   configopaque.String `mapstructure:"token"`
	Timeout time.Duration       `mapstructure:"timeout"`
	Limits  LimitsConfig        `mapstructure:"limits"`
	Filters FiltersConfig       `mapstructure:"filters"`
}

type LimitsConfig struct {
	MaxMetricsPerHost int `mapstructure:"max_metrics_per_host"`
	ItemsPerRequest   int `mapstructure:"items_per_request"`
}

type FiltersConfig struct {
	HostIncludeRegex    string `mapstructure:"host_include_regex"`
	HostExcludeRegex    string `mapstructure:"host_exclude_regex"`
	ItemKeyIncludeRegex string `mapstructure:"item_key_include_regex"`
	ItemKeyExcludeRegex string `mapstructure:"item_key_exclude_regex"`
}

func createDefaultConfig() component.Config {
	return &Config{
		Mode:      "api",
		Metadata:  MetadataConfig{Enabled: true, InheritedHostTags: true, HostGroupsFormat: "both"},
		Streaming: StreamingConfig{Endpoint: "127.0.0.1:8081", MaxRequestBodySize: 10485760, Timeout: 30 * time.Second},
		Schedule: ScheduleConfig{
			Jitter: 5 * time.Second,
			Jobs: JobsConfig{
				Discover: JobConfig{Enabled: true, RunOnStart: true, Interval: 5 * time.Minute, Timeout: time.Minute},
				Values:   JobConfig{Enabled: true, Interval: 30 * time.Second, Timeout: 20 * time.Second},
			},
		},
		Prom: PromConfig{Prefix: "zabbix_"},
		Zabbix: ZabbixConfig{
			Timeout: 30 * time.Second,
			Limits:  LimitsConfig{MaxMetricsPerHost: 1000, ItemsPerRequest: 1000},
		},
	}
}

func (c *Config) Clone() *Config {
	clone := *c
	clone.Metadata.InventoryFields = append([]string(nil), c.Metadata.InventoryFields...)
	if c.Prom.ConstLabels != nil {
		clone.Prom.ConstLabels = make(map[string]string, len(c.Prom.ConstLabels))
		for key, value := range c.Prom.ConstLabels {
			clone.Prom.ConstLabels[key] = value
		}
	}
	return &clone
}

func (c *Config) ResolveEnv(getenv func(string) (string, bool)) error {
	if c.Mode == "streaming" && !c.Streaming.EnrichWithAPI {
		return nil
	}
	if value, ok := getenv("ZABBIX_URL"); ok {
		c.Zabbix.URL = value
	}
	if value, ok := getenv("ZABBIX_TOKEN"); ok {
		c.Zabbix.Token = configopaque.String(value)
	}
	if value, ok := getenv("ZABBIX_TIMEOUT"); ok {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid ZABBIX_TIMEOUT: %w", err)
		}
		c.Zabbix.Timeout = duration
	}
	if value, ok := getenv("MAX_METRICS_PER_HOST"); ok {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid MAX_METRICS_PER_HOST: %w", err)
		}
		c.Zabbix.Limits.MaxMetricsPerHost = limit
	}
	if value, ok := getenv("ZABBIX_ITEMS_PER_REQUEST"); ok {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid ZABBIX_ITEMS_PER_REQUEST: %w", err)
		}
		c.Zabbix.Limits.ItemsPerRequest = limit
	}
	return nil
}

func (c *Config) Validate() error {
	resolved := c.Clone()
	if err := resolved.ResolveEnv(os.LookupEnv); err != nil {
		return fmt.Errorf("resolve Zabbix receiver environment: %w", err)
	}
	return resolved.validateResolved()
}

func (c *Config) validateResolved() error {
	var errs []error
	switch c.Metadata.HostGroupsFormat {
	case "names", "flags", "both":
	default:
		errs = append(errs, errors.New("metadata.host_groups_format must be names, flags or both"))
	}
	for _, field := range c.Metadata.InventoryFields {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(field) {
			errs = append(errs, errors.New("metadata.inventory_fields must contain non-empty inventory field names"))
		}
	}
	if c.Mode != "api" && c.Mode != "streaming" {
		errs = append(errs, errors.New("mode must be api or streaming"))
	}
	if c.Mode == "streaming" {
		if _, port, err := net.SplitHostPort(c.Streaming.Endpoint); err != nil || port == "" {
			errs = append(errs, errors.New("streaming.endpoint must be host:port"))
		}
		if c.Streaming.MaxRequestBodySize <= 0 {
			errs = append(errs, errors.New("streaming.max_request_body_size must be positive"))
		}
		if c.Streaming.Timeout <= 0 {
			errs = append(errs, errors.New("streaming.timeout must be positive"))
		}
		if !metricNamePattern.MatchString(c.Prom.Prefix) {
			errs = append(errs, errors.New("prom.prefix must be a valid Prometheus metric name prefix"))
		}
		if !c.Streaming.EnrichWithAPI {
			return errors.Join(errs...)
		}
		if !c.Metadata.Enabled {
			errs = append(errs, errors.New("streaming.enrich_with_api requires metadata.enabled"))
		}
		if !c.Schedule.Jobs.Discover.Enabled {
			errs = append(errs, errors.New("streaming.enrich_with_api requires schedule.jobs.discover.enabled"))
		}
	}

	parsedURL, err := url.Parse(c.Zabbix.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		errs = append(errs, errors.New("zabbix.url must be an HTTP or HTTPS URL"))
	}
	if c.Zabbix.Token == "" {
		errs = append(errs, errors.New("zabbix.token must not be empty"))
	}
	if c.Schedule.Jitter < 0 {
		errs = append(errs, errors.New("schedule.jitter must not be negative"))
	}
	validateJob := func(name string, job JobConfig) {
		if !job.Enabled {
			return
		}
		if job.Interval <= 0 {
			errs = append(errs, fmt.Errorf("schedule.jobs.%s.interval must be positive", name))
		}
		if job.Timeout <= 0 {
			errs = append(errs, fmt.Errorf("schedule.jobs.%s.timeout must be positive", name))
		}
	}
	validateJob("discover", c.Schedule.Jobs.Discover)
	if c.Mode != "streaming" {
		validateJob("values", c.Schedule.Jobs.Values)
	}
	if !c.Schedule.Jobs.Discover.Enabled && !c.Schedule.Jobs.Values.Enabled {
		errs = append(errs, errors.New("schedule.jobs must enable discover or values"))
	}
	if !metricNamePattern.MatchString(c.Prom.Prefix) {
		errs = append(errs, errors.New("prom.prefix must be a valid Prometheus metric name prefix"))
	}
	if c.Zabbix.Timeout <= 0 {
		errs = append(errs, errors.New("zabbix.timeout must be positive"))
	}
	if c.Zabbix.Limits.MaxMetricsPerHost <= 0 {
		errs = append(errs, errors.New("zabbix.limits.max_metrics_per_host must be positive"))
	}
	if c.Zabbix.Limits.ItemsPerRequest <= 0 {
		errs = append(errs, errors.New("zabbix.limits.items_per_request must be positive"))
	}
	for _, filter := range []struct {
		name  string
		value string
	}{
		{name: "host_include_regex", value: c.Zabbix.Filters.HostIncludeRegex},
		{name: "host_exclude_regex", value: c.Zabbix.Filters.HostExcludeRegex},
		{name: "item_key_include_regex", value: c.Zabbix.Filters.ItemKeyIncludeRegex},
		{name: "item_key_exclude_regex", value: c.Zabbix.Filters.ItemKeyExcludeRegex},
	} {
		if _, err := regexp.Compile(filter.value); err != nil {
			errs = append(errs, fmt.Errorf("zabbix.filters.%s is invalid: %w", filter.name, err))
		}
	}

	return errors.Join(errs...)
}
