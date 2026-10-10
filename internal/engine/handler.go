package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"gw-base/internal/config"
	"gw-base/internal/upstream"
)

const envPrefix = "env."

// handler serves one endpoint: validate -> call upstream -> map response.
type handler struct {
	ep         config.Endpoint
	up         *upstream.Upstream
	log        *slog.Logger
	maxBody    int64
	bodyParams bool
}

func newHandler(ep config.Endpoint, maxBody int64, logger *slog.Logger) (*handler, error) {
	if err := checkTemplates(ep); err != nil {
		return nil, err
	}
	if err := checkMessages(ep); err != nil {
		return nil, err
	}
	up, err := upstream.New(ep.Method, ep.Upstream, logger)
	if err != nil {
		return nil, err
	}
	h := &handler{ep: ep, up: up, log: logger.With("endpoint", ep.Name), maxBody: maxBody}
	for _, p := range ep.Params {
		if p.In == "body" {
			h.bodyParams = true
		}
	}
	return h, nil
}

// checkTemplates fails at startup on {{typos}} instead of at request time.
func checkTemplates(ep config.Endpoint) error {
	declared := make(map[string]bool, len(ep.Params))
	for _, p := range ep.Params {
		declared[p.Name] = true
	}
	for field, v := range ep.Upstream.Body {
		_, err := mapStrings(v, func(s string) (string, error) {
			for _, key := range placeholders(s) {
				if name, ok := strings.CutPrefix(key, envPrefix); ok {
					if _, set := os.LookupEnv(name); !set {
						return "", fmt.Errorf("env var %q is not set", name)
					}
				} else if !declared[key] {
					return "", fmt.Errorf("unknown param %q", key)
				}
			}
			return s, nil
		})
		if err != nil {
			return fmt.Errorf("upstream.body.%s: %w", field, err)
		}
	}
	return nil
}

// checkMessages makes sure business error messages only use {{ $.json.paths }}.
func checkMessages(ep config.Endpoint) error {
	for i, be := range ep.BusinessErrors {
		for _, key := range placeholders(be.Message) {
			if !strings.HasPrefix(key, "$.") {
				return fmt.Errorf("business_errors[%d]: message placeholder %q must be a $.json.path", i, key)
			}
		}
	}
	return nil
}

func (h *handler) serve(c *gin.Context) {
	params, err := h.collectParams(c)
	if err != nil {
		fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	fields, err := h.buildFields(params)
	if err != nil {
		h.log.Error("building upstream body failed", "error", err)
		fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	raw, status, err := h.up.Call(c.Request.Context(), fields)
	if err != nil {
		h.log.Error("upstream call failed", "error", err)
		fail(c, http.StatusBadGateway, "UPSTREAM_ERROR", "upstream call failed")
		return
	}

	doc, jsonErr := decodeJSON(raw)

	// Business errors win over transport-level checks: an upstream may report
	// its error in the body of a 200 as well as in the body of a 500.
	if jsonErr == nil {
		if be := h.matchBusinessError(doc); be != nil {
			msg, _ := render(be.Message, func(key string) (string, bool) {
				v, _ := lookupPath(doc, key)
				return stringify(v), true // a missing path renders empty
			})
			h.log.Warn("upstream business error", "upstream_status", status, "code", be.Code)
			fail(c, be.Status, be.Code, msg)
			return
		}
	}

	if status < 200 || status > 299 {
		h.log.Error("upstream returned error status", "status", status)
		fail(c, http.StatusBadGateway, "UPSTREAM_ERROR", fmt.Sprintf("upstream returned status %d", status))
		return
	}
	if jsonErr != nil {
		h.log.Error("upstream returned invalid JSON", "error", jsonErr)
		fail(c, http.StatusBadGateway, "UPSTREAM_ERROR", "invalid upstream response")
		return
	}

	out, err := h.mapResponse(doc)
	if err != nil {
		h.log.Error("mapping upstream response failed", "error", err)
		fail(c, http.StatusBadGateway, "UPSTREAM_ERROR", "invalid upstream response")
		return
	}
	succeed(c, h.ep.Response.Status, out)
}

// collectParams reads every declared param from its location and validates it.
func (h *handler) collectParams(c *gin.Context) (map[string]string, error) {
	var body map[string]any
	if h.bodyParams {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBody)
		dec := json.NewDecoder(c.Request.Body)
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			return nil, errors.New("request body must be a valid JSON object")
		}
	}

	out := make(map[string]string, len(h.ep.Params))
	for _, p := range h.ep.Params {
		var val string
		var err error
		switch p.In {
		case "body":
			if raw, found := body[p.Name]; found && raw != nil {
				val, err = fromJSON(p, raw)
			}
		case "query":
			val, err = fromText(p, c.Query(p.Name))
		case "path":
			val, err = fromText(p, c.Param(p.Name))
		case "header":
			val, err = fromText(p, c.GetHeader(p.Name))
		}
		if err != nil {
			return nil, err
		}

		if val == "" {
			if p.Required {
				return nil, fmt.Errorf("%s is required", p.Name)
			}
			continue
		}
		if p.Re != nil && !p.Re.MatchString(val) {
			return nil, fmt.Errorf("%s has an invalid format", p.Name) // never echo the value back
		}
		out[p.Name] = val
	}
	return out, nil
}

// buildFields renders upstream.body templates; nil means "send no body".
func (h *handler) buildFields(params map[string]string) (map[string]any, error) {
	if len(h.ep.Upstream.Body) == 0 {
		return nil, nil
	}
	lookup := func(key string) (string, bool) {
		if name, ok := strings.CutPrefix(key, envPrefix); ok {
			return os.LookupEnv(name)
		}
		return params[key], true // optional params that were not sent render empty
	}

	fields := make(map[string]any, len(h.ep.Upstream.Body))
	for name, tpl := range h.ep.Upstream.Body {
		v, err := mapStrings(tpl, func(s string) (string, error) { return render(s, lookup) })
		if err != nil {
			return nil, fmt.Errorf("upstream.body.%s: %w", name, err)
		}
		fields[name] = v
	}
	return fields, nil
}

// matchBusinessError returns the first business error whose condition holds.
func (h *handler) matchBusinessError(doc any) *config.BusinessError {
	for i := range h.ep.BusinessErrors {
		be := &h.ep.BusinessErrors[i]
		v, ok := lookupPath(doc, be.When.Path)
		if !ok {
			continue // a missing value never matches
		}
		got := stringify(v)
		if (be.When.Equals != nil && got == *be.When.Equals) ||
			(be.When.NotEquals != nil && got != *be.When.NotEquals) {
			return be
		}
	}
	return nil
}

// mapResponse builds the client payload from response.map.
func (h *handler) mapResponse(doc any) (map[string]any, error) {
	out := make(map[string]any, len(h.ep.Response.Map))
	for field, m := range h.ep.Response.Map {
		v, _ := lookupPath(doc, m.Path) // a missing path becomes null
		if m.Decode == "base64" && v != nil {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("field %q: cannot base64-decode a non-string value", field)
			}
			dec, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", field, err)
			}
			v = string(dec)
		}
		out[field] = v
	}
	return out, nil
}

func decodeJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep large numbers intact
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}
