package config

import (
	"regexp"
	"time"
)

type Config struct {
	SchemaVersion int        `yaml:"schema_version"`
	Service       Service    `yaml:"service"`
	Mode          string     `yaml:"mode"` // "pure" | "hybrid"
	Server        Server     `yaml:"server"`
	Logging       Logging    `yaml:"logging"`
	Endpoints     []Endpoint `yaml:"endpoints"`
}

type Service struct {
	Name     string            `yaml:"name"`
	Provider string            `yaml:"provider"`
	Domain   string            `yaml:"domain"`
	Version  string            `yaml:"version"`
	Labels   map[string]string `yaml:"labels"`
}

type Server struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
	MaxBodyBytes    int64         `yaml:"max_body_bytes"`
}

type Logging struct {
	FilePath string   `yaml:"filePath"`
	FileName string   `yaml:"fileName"`
	Encoding string   `yaml:"encoding"`
	Level    string   `yaml:"level"`
	Logger   string   `yaml:"logger"` // logger backend; default "slog"
	Console  bool     `yaml:"console"`
	Rotating Rotating `yaml:"rotating"`
}

type Rotating struct {
	MaxSize    int  `yaml:"maxSize"` // MB
	MaxBackups int  `yaml:"maxBackups"`
	MaxAge     int  `yaml:"maxAge"` // days
	Compress   bool `yaml:"compress"`
}

type Endpoint struct {
	Name           string          `yaml:"name"`
	Method         string          `yaml:"method"`
	Path           string          `yaml:"path"`
	Params         []Param         `yaml:"params"`
	Upstream       Upstream        `yaml:"upstream"`
	Response       Response        `yaml:"response"`
	BusinessErrors []BusinessError `yaml:"business_errors"`
}

// Param types; an unset type means TypeString.
const (
	TypeString  = "string"
	TypeBoolean = "boolean"
	TypeInteger = "integer"
	TypeNumber  = "number"
)

type Param struct {
	Name     string         `yaml:"name"`
	In       string         `yaml:"in"`   // body | query | path | header
	Type     string         `yaml:"type"` // string (default) | boolean | integer | number
	Required bool           `yaml:"required"`
	Pattern  string         `yaml:"pattern"` // string params only
	Re       *regexp.Regexp `yaml:"-"`       // compiled Pattern, set by Load
}

type Upstream struct {
	BaseURL    string            `yaml:"base_url"`
	BaseURLEnv string            `yaml:"base_url_env"`
	Method     string            `yaml:"method"` // GET | POST (default) | PUT | PATCH | DELETE
	Timeout    time.Duration     `yaml:"timeout"`
	Format     string            `yaml:"format"` // body encoding: json (default) | form (urlencoded) | multipart (form-data)
	Auth       *Auth             `yaml:"auth"`
	Headers    map[string]string `yaml:"headers"` // extra request headers; values may use {{env.NAME}}
	Jwe        Jwe               `yaml:"jwe"`
	Path       string            `yaml:"path"` // may contain {{param}}, {{env.NAME}} and {{ now:... }} placeholders
	Body       map[string]any    `yaml:"body"` // nested objects/lists allowed; strings may contain {{param}} and {{env.NAME}}
}

// Jwe configures JSON Web Encryption for {{ jwe:param }} placeholders in
// upstream.body. Exactly one of PublicKeyFile / PublicKeyEnv must be set when
// any jwe: placeholder is used. Alg and Enc default to ECDH-ES+A256KW / A256GCM.
type Jwe struct {
	PublicKeyFile string `yaml:"public_key_file"` // path to PEM public key
	PublicKeyEnv  string `yaml:"public_key_env"`  // env var holding PEM public key
	Alg           string `yaml:"alg"`             // key management algorithm
	Enc           string `yaml:"enc"`             // content encryption algorithm
}

type Auth struct {
	Type     string `yaml:"type"` // basic
	Username Secret `yaml:"username"`
	Password Secret `yaml:"password"`

	Script       string            `yaml:"script,omitempty"`
	ScriptIsFile bool              `yaml:"script_is_file,omitempty"`
	Cache        map[string]string `yaml:"cache,omitempty"`
}

type Secret struct {
	Source string `yaml:"source"` // "env"
	Value  string `yaml:"value"`  // env var name
}

// PathStatus is the when.path (and message placeholder) for the upstream's HTTP status.
const PathStatus = "@status"

// BusinessError turns an upstream answer into an error for the client.
// It is checked on every JSON answer, whatever the upstream's HTTP status.
type BusinessError struct {
	When    When   `yaml:"when"`
	Status  int    `yaml:"status"`  // client-facing HTTP status (4xx/5xx, default 422)
	Code    string `yaml:"code"`    // client-facing error code
	Message string `yaml:"message"` // may contain {{ $.json.path }} templates
}

// When matches on one value of the upstream answer: a JSON path ("$.a.b") or
// the HTTP status ("@status"). Set exactly one of Equals / NotEquals; a path
// that is missing from the answer never matches.
type When struct {
	Path      string  `yaml:"path"`
	Equals    *string `yaml:"equals"`
	NotEquals *string `yaml:"not_equals"`
}

type Response struct {
	Status int                     `yaml:"status"`
	Map    map[string]FieldMapping `yaml:"map"`
}

// FieldMapping accepts either:
//
//	key: "$.json.path"
//	key: { $decode: base64, path: "$.json.path" }
type FieldMapping struct {
	Path   string
	Decode string // "" or "base64"
}
