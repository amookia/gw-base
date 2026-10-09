package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

func (s Secret) Resolve() (string, error) {
	switch s.Source {
	case "env":
		v, ok := os.LookupEnv(s.Value)
		if !ok {
			return "", fmt.Errorf("env var %q is not set", s.Value)
		}
		return v, nil
	default:
		return "", fmt.Errorf("unsupported secret source %q", s.Source)
	}
}

func (f *FieldMapping) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Decode(&f.Path)
	case yaml.MappingNode:
		var aux struct {
			Decode string `yaml:"$decode"`
			Path   string `yaml:"path"`
		}
		if err := n.Decode(&aux); err != nil {
			return err
		}
		if aux.Path == "" {
			return fmt.Errorf("line %d: mapping object requires 'path'", n.Line)
		}
		if aux.Decode != "" && aux.Decode != "base64" {
			return fmt.Errorf("line %d: unsupported $decode %q", n.Line, aux.Decode)
		}
		f.Path, f.Decode = aux.Path, aux.Decode
		return nil
	default:
		return fmt.Errorf("line %d: mapping must be a string or object", n.Line)
	}
}

// Addr is the listen address; the PORT env var overrides server.port.
func (s Server) Addr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":" + strconv.Itoa(s.Port)
}

// Load reads, validates and applies defaults to the service YAML.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true) // fail on typos / unknown keys
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
