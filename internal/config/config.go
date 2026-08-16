package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	System    SystemConfig    `yaml:"system"`
	Container ContainerConfig `yaml:"container"`
	Services  []ServiceConfig `yaml:"services"`
}

type ServerConfig struct {
	Addr     string   `yaml:"addr"`
	Interval Duration `yaml:"interval"`
}

func (s ServerConfig) DefaultInterval() Duration { return s.Interval }

type SystemConfig struct {
	Enabled       bool     `yaml:"enabled"`
	Disks         []string `yaml:"disks"`
	NetInterfaces []string `yaml:"netInterfaces"`
}

type ContainerConfig struct {
	Enabled    bool              `yaml:"enabled"`
	Endpoint   string            `yaml:"endpoint"`
	Containers []ContainerFilter `yaml:"containers"`
}

type ContainerFilter struct {
	Name  string `yaml:"name"`
	Label string `yaml:"label"`
}

type ServiceConfig struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"` // http | tcp | udp
	URL        string            `yaml:"url"`
	Address    string            `yaml:"address"`
	Timeout    Duration          `yaml:"timeout"`
	Interval   Duration          `yaml:"interval"`
	Method     string            `yaml:"method"`
	Headers    map[string]string `yaml:"headers"`
	Payload    string            `yaml:"payload"`
	PayloadB64 string            `yaml:"payloadBase64"`
	Expect     string            `yaml:"expect"`
	Extract    map[string]string `yaml:"extract"`
	Container  *ContainerRef     `yaml:"container"`
}

func (s *ServiceConfig) Defaults(g Intervaler) {
	if s.Method == "" {
		s.Method = "GET"
	}
	if s.Timeout == 0 {
		s.Timeout = Duration(5 * time.Second)
	}
	if s.Interval == 0 {
		s.Interval = g.DefaultInterval()
	}
}

func (s *ServiceConfig) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("service name is required")
	}
	switch s.Type {
	case "http":
		if s.URL == "" {
			return fmt.Errorf("service %q: url is required for http", s.Name)
		}
	case "tcp", "udp":
		if s.Address == "" {
			return fmt.Errorf("service %q: address is required for %s", s.Name, s.Type)
		}
	default:
		return fmt.Errorf("service %q: unsupported type %q (http|tcp|udp)", s.Name, s.Type)
	}
	return nil
}

type ContainerRef struct {
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled"`
}

func (c *ContainerRef) IsEnabled() bool {
	if c == nil || c.Name == "" {
		return false
	}
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// Intervaler is implemented by the collector to provide the default interval.
type Intervaler interface {
	DefaultInterval() Duration
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Server.Interval == 0 {
		cfg.Server.Interval = Duration(5 * time.Second)
	}
	return cfg, nil
}
