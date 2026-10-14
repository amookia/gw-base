package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gw-base/internal/config"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// fakeUpstream records what the engine sends and answers with a canned reply.
type fakeUpstream struct {
	calls int
	req   http.Request
	body  map[string]string
}

// newTestEngine loads the real test.yaml and points its upstream at a fakeUpstream.
func newTestEngine(t *testing.T, status int, reply string) (*Engine, *fakeUpstream) {
	t.Helper()

	fake := &fakeUpstream{body: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.calls++
		fake.req = *r
		_ = json.NewDecoder(r.Body).Decode(&fake.body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("INQUIRY_BASE_URL", srv.URL)
	t.Setenv("INQUIRY_USERNAME", "user")
	t.Setenv("INQUIRY_PASSWORD", "pass")

	cfg, err := config.Load("../../test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return eng, fake
}

func post(eng *Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/inquiry", strings.NewReader(body))
	rec := httptest.NewRecorder()
	eng.router.ServeHTTP(rec, req)
	return rec
}

func reply(message string) string {
	return `{"result":{"data":{"getEstelam3Response":{"return":{` +
		`"birthDate":"13800415","name":"` + b64("علی") + `","family":"` + b64("رضایی") + `",` +
		`"gender":"M","officeName":"Tehran","shenasnameSeri":"` + b64("12") + `",` +
		`"shenasnameSerial":"345","message":"` + message + `"}}}}}`
}

func TestInquirySuccess(t *testing.T) {
	eng, up := newTestEngine(t, 200, reply(""))

	rec := post(eng, `{"nationalCode":"0012345678","birthDate":"13800415"}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var env struct {
		IsSuccess bool           `json:"isSuccess"`
		Result    map[string]any `json:"result"`
		Error     any            `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.IsSuccess || env.Error != nil {
		t.Fatalf("not a success envelope: %s", rec.Body)
	}
	got := env.Result
	if got["name"] != "علی" || got["family"] != "رضایی" || got["gender"] != "M" {
		t.Fatalf("unexpected payload: %v", got)
	}

	if up.calls != 1 || up.req.URL.Path != "/astannix/GetEstelam3" || up.req.Method != http.MethodPost {
		t.Fatalf("unexpected upstream request: calls=%d %s %s", up.calls, up.req.Method, up.req.URL.Path)
	}
	if u, p, ok := up.req.BasicAuth(); !ok || u != "user" || p != "pass" {
		t.Fatalf("basic auth not sent: %v %v %v", u, p, ok)
	}
	if up.body["nin"] != "0012345678" || up.body["birthDate"] != "13800415" {
		t.Fatalf("templates not rendered: %v", up.body)
	}
}

func TestBusinessError(t *testing.T) {
	eng, _ := newTestEngine(t, 200, reply("err.record.not.found"))

	rec := post(eng, `{"nationalCode":"0012345678","birthDate":"13800415"}`)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"PERSON_NOT_FOUND"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "err.record.not.found") {
		t.Fatalf("message template not rendered: %s", rec.Body)
	}
}

func TestInvalidRequestNeverReachesUpstream(t *testing.T) {
	eng, up := newTestEngine(t, 200, reply(""))

	for name, body := range map[string]string{
		"bad pattern":     `{"nationalCode":"123","birthDate":"13800415"}`,
		"missing field":   `{"nationalCode":"0012345678"}`,
		"not json":        `nope`,
		"nested value":    `{"nationalCode":{"a":1},"birthDate":"13800415"}`,
		"empty body":      ``,
		"null field":      `{"nationalCode":null,"birthDate":"13800415"}`,
		"wrong date size": `{"nationalCode":"0012345678","birthDate":"1380"}`,
	} {
		if rec := post(eng, body); rec.Code != 400 {
			t.Errorf("%s: status = %d, body = %s", name, rec.Code, rec.Body)
		}
	}
	if up.calls != 0 {
		t.Fatalf("upstream was called %d times", up.calls)
	}
}

func TestUpstreamFailure(t *testing.T) {
	eng, _ := newTestEngine(t, 500, `boom`)
	if rec := post(eng, `{"nationalCode":"0012345678","birthDate":"13800415"}`); rec.Code != 502 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestNestedBodyTemplates(t *testing.T) {
	t.Setenv("GW_TEST_SECRET", "s3")
	ep := config.Endpoint{
		Name:   "nested",
		Method: http.MethodPost,
		Params: []config.Param{{Name: "id", In: "body"}},
		Upstream: config.Upstream{
			BaseURL: "http://upstream.invalid", Path: "/p", Format: "json", Timeout: time.Second,
			Body: map[string]any{
				"xx":    map[string]any{"x1": "{{id}}"},
				"items": []any{map[string]any{"id": "{{ id }}", "key": "{{env.GW_TEST_SECRET}}"}, "plain", 7},
			},
		},
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	h, err := newHandler(ep, 1<<20, newBuiltins(), quiet)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := h.buildFields(map[string]string{"id": "42"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(fields)
	want := `{"items":[{"id":"42","key":"s3"},"plain",7],"xx":{"x1":"42"}}`
	if string(got) != want {
		t.Errorf("body = %s, want %s", got, want)
	}

	ep.Upstream.Body = map[string]any{"a": []any{map[string]any{"b": "{{nope}}"}}}
	if _, err := newHandler(ep, 1<<20, newBuiltins(), quiet); err == nil {
		t.Error("unknown param inside a nested body should fail at startup")
	}
}

const businessYAML = `
schema_version: 1
service: { name: svc }
endpoints:
  - name: biz
    method: POST
    path: /v1/biz
    params:
      - { name: id, in: body, required: true }
    upstream:
      base_url: UPSTREAM_URL
      path: /p
      body: { id: "{{id}}" }
    response:
      map: { value: "$.result.data.Result.Value" }
    business_errors:
      - when: { path: "@status", equals: 401 }
        status: 502
        code: UPSTREAM_UNAUTHORIZED
        message: "provider rejected the credentials"
      - when: { path: "@status", equals: 429 }
        status: 503
        code: RATE_LIMITED
        message: "slow down ({{ @status }})"
      - when: { path: "$.result.data.Result.No", equals: "-1" }
        status: 400
        code: UPSTREAM_ERROR
        message: "{{ $.result.data.Result.Message }}"
      - when: { path: "$.result.data.Result.Level", not_equals: "ok" }
        status: 500
        code: BAD_LEVEL
        message: "level is {{ $.result.data.Result.Level }}"
`

func TestBusinessErrors(t *testing.T) {
	result := func(fields string) string { return `{"result":{"data":{"Result":{` + fields + `}}}}` }

	for _, tc := range []struct {
		name       string
		upstream   int
		body       string
		wantStatus int
		wantBody   string
	}{
		{"No equals -1 on a 200", 200, result(`"No":-1,"Message":"bad thing","Level":"ok"`),
			400, `"code":"UPSTREAM_ERROR","message":"bad thing"`},
		{"same error inside an HTTP 500", 500, result(`"No":-1,"Message":"bad thing","Level":"ok"`),
			400, `"code":"UPSTREAM_ERROR","message":"bad thing"`},
		{"401 matched by @status, non-JSON body", 401, `denied`,
			502, `"code":"UPSTREAM_UNAUTHORIZED","message":"provider rejected the credentials"`},
		{"401 matched by @status, JSON body", 401, result(`"No":0,"Level":"ok"`),
			502, `"code":"UPSTREAM_UNAUTHORIZED"`},
		{"@status usable in a message", 429, `{}`,
			503, `"code":"RATE_LIMITED","message":"slow down (429)"`},
		{"plain HTTP 500 is still a 502", 500, `boom`,
			502, `"message":"upstream returned status 500"`},
		{"No not -1 passes through", 200, result(`"No":0,"Value":"v","Level":"ok"`),
			200, `"value":"v"`},
		{"not_equals fires on a different value", 200, result(`"No":0,"Level":"weird"`),
			500, `"code":"BAD_LEVEL","message":"level is weird"`},
		{"a missing path never matches", 200, result(`"No":0,"Value":"v"`),
			200, `"value":"v"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.upstream)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			path := filepath.Join(t.TempDir(), "svc.yaml")
			if err := os.WriteFile(path, []byte(strings.Replace(businessYAML, "UPSTREAM_URL", srv.URL, 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			eng, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/biz", strings.NewReader(`{"id":"7"}`))
			rec := httptest.NewRecorder()
			eng.router.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("got %d %s\nwant %d containing %s", rec.Code, rec.Body, tc.wantStatus, tc.wantBody)
			}
		})
	}
}

func TestLookupPath(t *testing.T) {
	doc, _ := decodeJSON([]byte(`{"a":{"b":[{"c":"x"},{"c":"y"}]},"n":12345678901234567890}`))

	for path, want := range map[string]string{"$.a.b[1].c": "y", "$.n": "12345678901234567890"} {
		if v, ok := lookupPath(doc, path); !ok || stringify(v) != want {
			t.Errorf("%s = %v, %v; want %s", path, v, ok, want)
		}
	}
	for _, path := range []string{"$.a.x", "$.a.b[2].c", "a.b", "$.a.b.c"} {
		if _, ok := lookupPath(doc, path); ok {
			t.Errorf("%s should not resolve", path)
		}
	}
}

func TestEnvelopeShape(t *testing.T) {
	eng, _ := newTestEngine(t, 200, reply(""))
	eng.router.GET("/boom", func(*gin.Context) { panic("kaboom") })

	for name, tc := range map[string]struct {
		method, path, body string
		status             int
		want               string
	}{
		"success": {http.MethodPost, "/v1/inquiry", `{"nationalCode":"0012345678","birthDate":"13800415"}`,
			200, `{"isSuccess":true,"result":{`},
		"validation error": {http.MethodPost, "/v1/inquiry", `{}`,
			400, `{"isSuccess":false,"result":null,"error":{"code":"INVALID_REQUEST","message":"nationalCode is required"}}`},
		"unknown route": {http.MethodGet, "/nope", ``,
			404, `{"isSuccess":false,"result":null,"error":{"code":"NOT_FOUND","message":"route not found"}}`},
		"wrong method": {http.MethodGet, "/v1/inquiry", ``,
			405, `{"isSuccess":false,"result":null,"error":{"code":"METHOD_NOT_ALLOWED","message":"method not allowed"}}`},
		"panic": {http.MethodGet, "/boom", ``,
			500, `{"isSuccess":false,"result":null,"error":{"code":"INTERNAL_ERROR","message":"internal error"}}`},
		"health": {http.MethodGet, "/healthz", ``,
			200, `{"isSuccess":true,"result":{"status":"ok"},"error":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			eng.router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("got %d %s\nwant %d containing %s", rec.Code, rec.Body, tc.status, tc.want)
			}
		})
	}
}

const typedYAML = `
schema_version: 1
service: { name: svc }
endpoints:
  - name: typed
    method: POST
    path: /v1/typed
    params:
      - { name: id,    in: body,  required: true }
      - { name: ok,    in: body,  required: true, type: boolean }
      - { name: age,   in: body,  type: integer }
      - { name: score, in: query, type: number }
    upstream:
      base_url: UPSTREAM_URL
      path: /p
      body: { id: "{{id}}", ok: "{{ok}}", age: "{{age}}", score: "{{score}}" }
    response:
      map: { id: "$.id" }
`

func TestTypedParams(t *testing.T) {
	var seen map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&seen)
		_, _ = io.WriteString(w, `{"id":"1"}`)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "svc.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(typedYAML, "UPSTREAM_URL", srv.URL, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, url, body, want string
		status                int
	}{
		{"all valid; false still counts as sent", "/v1/typed?score=1.5", `{"id":"7","ok":false,"age":30}`, ``, 200},
		{"boolean sent as text", "/v1/typed", `{"id":"7","ok":"true"}`, `ok must be a boolean`, 400},
		{"missing required boolean", "/v1/typed", `{"id":"7"}`, `ok is required`, 400},
		{"default type is string", "/v1/typed", `{"id":7,"ok":true}`, `id must be a string`, 400},
		{"integer rejects a fraction", "/v1/typed", `{"id":"7","ok":true,"age":30.5}`, `age must be an integer`, 400},
		{"integer rejects text", "/v1/typed", `{"id":"7","ok":true,"age":"30"}`, `age must be an integer`, 400},
		{"query number rejects text", "/v1/typed?score=abc", `{"id":"7","ok":true}`, `score must be a number`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen = nil
			rec := httptest.NewRecorder()
			eng.router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body)))
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("got %d %s, want %d containing %q", rec.Code, rec.Body, tc.status, tc.want)
			}
			if tc.status == 400 && seen != nil {
				t.Error("an invalid request must not reach the upstream")
			}
		})
	}

	// the valid case, again, to look at what the upstream received
	rec := httptest.NewRecorder()
	eng.router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/typed?score=1.5", strings.NewReader(`{"id":"7","ok":false,"age":30}`)))
	if seen["ok"] != "false" || seen["age"] != "30" || seen["score"] != "1.5" || seen["id"] != "7" {
		t.Errorf("upstream saw %v", seen)
	}
}

func TestResolveHeaders(t *testing.T) {
	t.Setenv("GW_TEST_TOKEN", "tok")
	got, err := resolveHeaders(map[string]string{"Authorization": "Bearer {{env.GW_TEST_TOKEN}}", "Accept": "*/*"})
	if err != nil || got["Authorization"] != "Bearer tok" || got["Accept"] != "*/*" {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := resolveHeaders(map[string]string{"X": "{{env.GW_TEST_UNSET_VAR}}"}); err == nil {
		t.Error("an unset env var must fail at startup")
	}
	if _, err := resolveHeaders(map[string]string{"X": "{{param}}"}); err == nil {
		t.Error("only {{env.NAME}} is allowed in headers")
	}
}

func TestTimeBuiltins(t *testing.T) {
	// built from time.Local, so the expected text holds on any machine
	instant := time.Date(2026, 10, 12, 9, 58, 0, 90_000_000, time.Local)
	ep := config.Endpoint{
		Name:   "ids",
		Method: http.MethodPost,
		Upstream: config.Upstream{
			BaseURL: "http://upstream.invalid", Path: "/p", Format: "json", Timeout: time.Second,
			Body: map[string]any{"requestId": "0632{{ now:yyyyMMddHHmmss }}{{ millis:6 }}", "day": "{{now:yyyy-MM-dd}}"},
		},
	}

	clock := newBuiltins()
	calls := 0
	clock.now = func() time.Time { calls++; return instant }
	h, err := newHandler(ep, 1<<20, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	fields, err := h.buildFields(nil)
	if err != nil {
		t.Fatal(err)
	}

	wantMillis := fmt.Sprintf("%06d", instant.UnixMilli()%1_000_000)
	if fields["requestId"] != "0632"+"20261012095800"+wantMillis || fields["day"] != "2026-10-12" {
		t.Errorf("fields = %v", fields)
	}
	if len(fields["requestId"].(string)) != len("0632")+14+6 {
		t.Errorf("requestId %q has the wrong length", fields["requestId"])
	}
	if calls != 1 {
		t.Errorf("clock read %d times for one request, want 1", calls)
	}
}

func TestBuiltinTemplatesAreValidatedAtStartup(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tpl := range []string{"{{ now: }}", "{{ now:yyyyMMddXX }}", "{{ millis:0 }}", "{{ millis:14 }}", "{{ millis:abc }}"} {
		ep := config.Endpoint{
			Name: "bad", Method: http.MethodPost,
			Upstream: config.Upstream{BaseURL: "http://u.invalid", Path: "/p", Timeout: time.Second,
				Body: map[string]any{"x": tpl}},
		}
		if _, err := newHandler(ep, 1<<20, newBuiltins(), quiet); err == nil {
			t.Errorf("%s should fail at startup", tpl)
		}
	}
}

const pathYAML = `
schema_version: 1
service: { name: svc }
endpoints:
  - name: rel
    method: POST
    path: /v1/rel
    params:
      - { name: id,   in: body, required: true }
      - { name: kind, in: body }
    upstream:
      base_url: UPSTREAM_URL
      method: GET
      path: /rel/{{id}}/{{ kind }}/{{env.GW_TEST_TENANT}}
    response:
      map: { ok: "$.ok" }
  - name: plain
    method: GET
    path: /v1/plain
    upstream:
      base_url: UPSTREAM_URL
      path: /plain
    response:
      map: { ok: "$.ok" }
`

func TestUpstreamPathParams(t *testing.T) {
	t.Setenv("GW_TEST_TENANT", "t1")
	var method, uri string
	var bodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, uri = r.Method, r.RequestURI
		b, _ := io.ReadAll(r.Body)
		bodyLen = len(b)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "svc.yaml")
	if err := os.WriteFile(path, []byte(strings.Replace(pathYAML, "UPSTREAM_URL", srv.URL, -1)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	call := func(m, url, body string) *httptest.ResponseRecorder {
		method, uri, bodyLen = "", "", 0
		rec := httptest.NewRecorder()
		eng.router.ServeHTTP(rec, httptest.NewRequest(m, url, strings.NewReader(body)))
		return rec
	}

	for name, tc := range map[string]struct {
		body       string
		status     int
		wantURI    string
		wantInBody string
	}{
		"plain values":          {`{"id":"123","kind":"x"}`, 200, "/rel/123/x/t1", ""},
		"slash and query chars": {`{"id":"a/b?x=1#f","kind":"x"}`, 200, "/rel/a%2Fb%3Fx=1%23f/x/t1", ""},
		"spaces and unicode":    {`{"id":"a b","kind":"\u0633"}`, 200, "/rel/a%20b/%D8%B3/t1", ""},
		"dot-dot is refused":    {`{"id":"..","kind":"x"}`, 400, "", "id has an invalid value"},
		"single dot is refused": {`{"id":"1","kind":"."}`, 400, "", "kind has an invalid value"},
		"missing path param":    {`{"id":"1"}`, 400, "", "kind is required"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := call(http.MethodPost, "/v1/rel", tc.body)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.wantInBody) {
				t.Fatalf("got %d %s", rec.Code, rec.Body)
			}
			if tc.status == 200 && (method != "GET" || uri != tc.wantURI || bodyLen != 0) {
				t.Errorf("upstream saw %s %s (body %d bytes), want GET %s with no body", method, uri, bodyLen, tc.wantURI)
			}
			if tc.status == 400 && uri != "" {
				t.Errorf("a refused request must not reach the upstream (saw %s)", uri)
			}
		})
	}

	// no upstream.method: POST, whatever the client used (here GET)
	if rec := call(http.MethodGet, "/v1/plain", ""); rec.Code != 200 || method != "POST" || uri != "/plain" {
		t.Errorf("got %d, upstream saw %s %s, want POST /plain", rec.Code, method, uri)
	}
}

func TestUpstreamPathIsValidatedAtStartup(t *testing.T) {
	t.Setenv("GW_TEST_TENANT", "t1")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, p := range []string{"/x/{{nope}}", "/x/{{env.GW_TEST_UNSET_VAR}}", "/x/{{ jwe:id }}", "/x/{{ now:bogus }}"} {
		ep := config.Endpoint{
			Name: "bad", Method: http.MethodPost,
			Params:   []config.Param{{Name: "id", In: "body"}},
			Upstream: config.Upstream{BaseURL: "http://u.invalid", Path: p, Timeout: time.Second},
		}
		if _, err := newHandler(ep, 1<<20, newBuiltins(), quiet); err == nil {
			t.Errorf("path %q should fail at startup", p)
		}
	}
}
