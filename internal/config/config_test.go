package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadYAML(t *testing.T, doc string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "svc.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

const nestedYAML = `
schema_version: 1
service: { name: svc }
endpoints:
  - name: nested
    method: POST
    path: /v1/x
    params:
      - { name: id, in: body, required: true }
    upstream:
      base_url: http://upstream.invalid
      path: /p
      format: %s
      body:
        plain: "{{id}}"
        count: 5
        xx: { x1: x2 }
        items:
          - { id: "{{id}}", tags: [a, b] }
          - { id: "2" }
    response:
      map:
        id: "$.id"
`

func TestNestedBodyLoads(t *testing.T) {
	cfg, err := loadYAML(t, strings.Replace(nestedYAML, "%s", "json", 1))
	if err != nil {
		t.Fatal(err)
	}
	body := cfg.Endpoints[0].Upstream.Body
	if xx, ok := body["xx"].(map[string]any); !ok || xx["x1"] != "x2" {
		t.Errorf("xx = %#v", body["xx"])
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 2 {
		t.Errorf("items = %#v", body["items"])
	}
}

func TestFormBodyMustBeFlat(t *testing.T) {
	_, err := loadYAML(t, strings.Replace(nestedYAML, "%s", "form", 1))
	if err == nil || !strings.Contains(err.Error(), "nested values need format: json") {
		t.Errorf("err = %v", err)
	}
}

const bizHead = `
schema_version: 1
service: { name: svc }
endpoints:
  - name: biz
    method: POST
    path: /v1/x
    upstream: { base_url: "http://u.invalid", path: /p }
    business_errors:
`

func TestBusinessErrorRules(t *testing.T) {
	ok := `      - { when: { path: "$.a", equals: -1 }, status: 400, code: E }` // unquoted numbers are fine
	if cfg, err := loadYAML(t, bizHead+ok); err != nil {
		t.Fatal(err)
	} else if be := cfg.Endpoints[0].BusinessErrors[0]; be.Status != 400 || *be.When.Equals != "-1" {
		t.Errorf("rule = %+v", be)
	}

	for name, tc := range map[string]struct{ rule, want string }{
		"both operators": {`      - { when: { path: "$.a", equals: "1", not_equals: "2" }, code: E }`, "exactly one of"},
		"no operator":    {`      - { when: { path: "$.a" }, code: E }`, "exactly one of"},
		"missing code":   {`      - { when: { path: "$.a", equals: "1" } }`, "code is required"},
		"bad path":       {`      - { when: { path: "a", equals: "1" }, code: E }`, "must start with $."},
		"2xx status":     {`      - { when: { path: "$.a", equals: "1" }, code: E, status: 200 }`, "must be 4xx or 5xx"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadYAML(t, bizHead+tc.rule); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}

	// the HTTP status can be matched too, with an unquoted number
	cfg, err := loadYAML(t, bizHead+`      - { when: { path: "@status", equals: 401 }, status: 502, code: E }`)
	if err != nil {
		t.Fatal(err)
	}
	if w := cfg.Endpoints[0].BusinessErrors[0].When; w.Path != PathStatus || *w.Equals != "401" {
		t.Errorf("when = %+v", w)
	}

	// status defaults to 422 and `equals: ""` is a valid condition
	cfg, err = loadYAML(t, bizHead+`      - { when: { path: "$.a", equals: "" }, code: E }`)
	if err != nil {
		t.Fatal(err)
	}
	if be := cfg.Endpoints[0].BusinessErrors[0]; be.Status != 422 || be.When.Equals == nil {
		t.Errorf("rule = %+v", be)
	}
}

const paramHead = `
schema_version: 1
service: { name: svc }
endpoints:
  - name: p
    method: POST
    path: /v1/x
    upstream: { base_url: "http://u.invalid", path: /p }
    params:
`

func TestParamTypes(t *testing.T) {
	cfg, err := loadYAML(t, paramHead+`
      - { name: a, in: body }
      - { name: b, in: body, required: true, type: boolean }
      - { name: c ,   in: query, type: integer }
`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range cfg.Endpoints[0].Params {
		got = append(got, p.Type)
	}
	if strings.Join(got, ",") != "string,boolean,integer" {
		t.Errorf("types = %v, want an unset type to default to string", got)
	}

	for name, tc := range map[string]struct{ param, want string }{
		"unknown type":       {`      - { name: a, in: body, type: date }`, "type must be"},
		"pattern on boolean": {`      - { name: a, in: body, type: boolean, pattern: '^x$' }`, "pattern only applies to type string"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadYAML(t, paramHead+tc.param); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
