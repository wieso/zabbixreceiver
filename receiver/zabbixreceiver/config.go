package zabbixreceiver

import (
	"errors"
	"fmt"
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
	Schedule ScheduleConfig `mapstructure:"schedule"`
	Prom     PromConfig     `mapstructure:"prom"`
	Zabbix   ZabbixConfig   `mapstructure:"zabbix"`
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
	if c.Prom.ConstLabels != nil {
		clone.Prom.ConstLabels = make(map[string]string, len(c.Prom.ConstLabels))
		for key, value := range c.Prom.ConstLabels {
			clone.Prom.ConstLabels[key] = value
		}
	}
	return &clone
}

func (c *Config) ResolveEnv(getenv func(string) (string, bool)) error {
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
	validateJob("values", c.Schedule.Jobs.Values)
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
