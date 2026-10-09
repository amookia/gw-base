// Package logging builds the application's *slog.Logger.
//
// It knows nothing about the application's config. The rest of the code only
// depends on *slog.Logger, so a different logger is added here, as a Backend,
// without touching anything else.
package logging

import (
	"fmt"
	"log/slog"
)

const defaultBackend = "slog"

// Options configures a logger.
type Options struct {
	Backend  string // "" selects "slog"; other backends are added with Register
	Level    string // debug | info | warn | error (default info)
	Encoding string // json (default) | text; how the backend formats records
	Console  bool   // also write to stdout
	File     File   // optional rotating file
}

// File is a rotating log file; an empty Name disables file output.
type File struct {
	Dir        string // default "."
	Name       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// Backend builds a logger from Options. Implementations can reuse OpenWriter
// and ParseLevel so every backend honors the same console/file/level options.
type Backend func(Options) (*slog.Logger, error)

var backends = map[string]Backend{defaultBackend: newSlog}

// Register makes a backend selectable through Options.Backend.
// Call it at startup, before New; it is not safe for concurrent use.
func Register(name string, b Backend) {
	if name == "" || b == nil {
		panic("logging: Register needs a name and a backend")
	}
	if _, dup := backends[name]; dup {
		panic(fmt.Sprintf("logging: backend %q already registered", name))
	}
	backends[name] = b
}

// New builds a logger with the selected backend.
func New(opts Options) (*slog.Logger, error) {
	name := opts.Backend
	if name == "" {
		name = defaultBackend
	}
	b, ok := backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown logger backend %q", name)
	}
	return b(opts)
}
