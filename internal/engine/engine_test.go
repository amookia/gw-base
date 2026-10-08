package engine

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
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
