package config

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var paramLocations = map[string]bool{"body": true, "query": true, "path": true, "header": true}

// normalize applies defaults and rejects configs that can't work.
func (c *Config) normalize() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d (want 1)", c.SchemaVersion)
	}
	if c.Service.Name == "" {
		return errors.New("service.name is required")
	}
	if c.Service.Version == "" {
		c.Service.Version = "0.1.0"
	}

	switch c.Mode {
	case "":
		c.Mode = "pure"
	case "pure", "hybrid":
	default:
		return fmt.Errorf("mode must be \"pure\" or \"hybrid\", got %q", c.Mode)
	}

	c.applyServerDefaults()

	if len(c.Endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}
	seen := make(map[string]bool, len(c.Endpoints))
	for i := range c.Endpoints {
		ep := &c.Endpoints[i]
		if err := ep.normalize(); err != nil {
			return fmt.Errorf("endpoint %q: %w", ep.Name, err)
		}
		route := ep.Method + " " + ep.Path
		if seen[route] {
			return fmt.Errorf("duplicate route %s", route)
		}
		seen[route] = true
	}
	return nil
}

func (c *Config) applyServerDefaults() {
	s := &c.Server
	if s.Port == 0 {
		s.Port = 8080
	}
	if s.ReadTimeout == 0 {
		s.ReadTimeout = 10 * time.Second
	}
	if s.WriteTimeout == 0 {
		s.WriteTimeout = 30 * time.Second
	}
	if s.ShutdownTimeout == 0 {
		s.ShutdownTimeout = 15 * time.Second
	}
	if s.MaxBodyBytes == 0 {
		s.MaxBodyBytes = 1 << 20
	}
}

func (e *Endpoint) normalize() error {
	if e.Name == "" {
		return errors.New("name is required")
	}
	e.Method = strings.ToUpper(e.Method)
	switch e.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("unsupported method %q", e.Method)
	}
	if !strings.HasPrefix(e.Path, "/") {
		return fmt.Errorf("path %q must start with /", e.Path)
	}

	for i := range e.Params {
		p := &e.Params[i]
		if p.Name == "" {
			return errors.New("param name is required")
		}
		if !paramLocations[p.In] {
			return fmt.Errorf("param %q: 'in' must be body, query, path or header", p.Name)
		}
		switch p.Type {
		case "":
			p.Type = TypeString
		case TypeString, TypeBoolean, TypeInteger, TypeNumber:
		default:
			return fmt.Errorf("param %q: type must be string, boolean, integer or number, got %q", p.Name, p.Type)
		}
		if p.Pattern != "" {
			if p.Type != TypeString {
				return fmt.Errorf("param %q: pattern only applies to type string", p.Name)
			}
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return fmt.Errorf("param %q: bad pattern: %w", p.Name, err)
			}
			p.Re = re
		}
	}

	if _, err := e.Upstream.URL(); err != nil {
		return err
	}
	switch e.Upstream.Format {
	case "":
		e.Upstream.Format = "json"
	case "json", "form", "multipart":
	default:
		return fmt.Errorf("upstream.format must be json, form or multipart, got %q", e.Upstream.Format)
	}
	if err := checkBody(e.Upstream.Body, e.Upstream.Format != "json"); err != nil {
		return fmt.Errorf("upstream.body.%w", err)
	}
	if e.Upstream.Timeout == 0 {
		e.Upstream.Timeout = 10 * time.Second
	}

	if e.Response.Status == 0 {
		e.Response.Status = http.StatusOK
	}
	for field, m := range e.Response.Map {
		if !strings.HasPrefix(m.Path, "$.") {
			return fmt.Errorf("response.map.%s: path %q must start with $.", field, m.Path)
		}
	}

	for i := range e.BusinessErrors {
		if err := e.BusinessErrors[i].normalize(); err != nil {
			return fmt.Errorf("business_errors[%d]: %w", i, err)
		}
	}
	return nil
}

func (be *BusinessError) normalize() error {
	if be.When.Path != PathStatus && !strings.HasPrefix(be.When.Path, "$.") {
		return fmt.Errorf("when.path %q must start with $. or be %s", be.When.Path, PathStatus)
	}
	if (be.When.Equals == nil) == (be.When.NotEquals == nil) {
		return errors.New("set exactly one of when.equals / when.not_equals")
	}
	if be.Code == "" {
		return errors.New("code is required")
	}
	if be.Status == 0 {
		be.Status = http.StatusUnprocessableEntity
	}
	if be.Status < 400 || be.Status > 599 {
		return fmt.Errorf("status %d must be 4xx or 5xx", be.Status)
	}
	return nil
}

// URL returns the full upstream URL; base_url_env overrides base_url when set.
func (u Upstream) URL() (string, error) {
	base := u.BaseURL
	if u.BaseURLEnv != "" {
		if v := os.Getenv(u.BaseURLEnv); v != "" {
			base = v
		}
	}
	if base == "" {
		return "", errors.New("upstream.base_url (or base_url_env) is required")
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(u.Path, "/"), nil
}

// checkBody accepts strings, numbers, bools, null, lists and objects; form bodies must be flat.
func checkBody(fields map[string]any, flat bool) error {
	for name, v := range fields {
		if flat {
			switch v.(type) {
			case map[string]any, []any:
				return fmt.Errorf("%s: nested values need format: json", name)
			}
		}
		if err := checkValue(v); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func checkValue(v any) error {
	switch t := v.(type) {
	case nil, string, bool, int, int64, uint64, float64:
		return nil
	case []any:
		for i, e := range t {
			if err := checkValue(e); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
		return nil
	case map[string]any:
		for k, e := range t {
			if err := checkValue(e); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported value of type %T (object keys must be strings)", v)
	}
}
