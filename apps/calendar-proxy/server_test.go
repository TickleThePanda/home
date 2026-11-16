package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef"

// newTestServer serves a synthetic upstream feed and captures the proxy's
// logs. fail makes the upstream return 500.
func newTestServer(t *testing.T, fail *bool) (*httptest.Server, *server, *bytes.Buffer) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if *fail {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		w.Write(calendar(event("a", "Planning", "DTSTART:20261120T100000Z", "DTEND:20261120T110000Z")))
	}))
	t.Cleanup(upstream.Close)

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := "feeds:\n  - name: test\n    token: " + testToken + "\n    source: " + upstream.URL + "/secret-path.ics\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	s := newServer(cfgPath)
	clock := now
	s.now = func() time.Time { return clock }
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return srv, s, &logs
}

func get(t *testing.T, url string, header ...string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestRequestLogging(t *testing.T) {
	fail := false
	srv, s, logs := newTestServer(t, &fail)
	feedURL := srv.URL + "/calendar/" + testToken + ".ics"

	first := get(t, feedURL, "Cf-Connecting-Ip", "203.0.113.7", "User-Agent", "test-agent")
	get(t, feedURL)
	get(t, feedURL, "If-None-Match", first.Header.Get("ETag"))
	get(t, srv.URL+"/calendar/"+strings.Repeat("f", 32)+".ics")

	// Expire the cache with the upstream down: the stale copy is served.
	fail = true
	later := now.Add(cacheTTL + time.Minute)
	s.now = func() time.Time { return later }
	get(t, feedURL)

	out := logs.String()
	for _, want := range []string{
		`msg="feed refreshed" feed=test`, "changed=true",
		`msg=request feed=test status=200`, "cache=miss", "client=203.0.113.7", "user_agent=test-agent",
		"cache=hit", "status=304",
		`msg=request feed=unknown status=404`,
		`msg="refresh failed, serving stale copy"`, "cache=stale",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs lack %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{testToken, "secret-path"} {
		if strings.Contains(out, secret) {
			t.Errorf("logs contain secret %q:\n%s", secret, out)
		}
	}

	metrics := httptest.NewRecorder()
	s.serveMetrics(metrics, nil)
	for _, want := range []string{
		`calendar_proxy_requests_total{feed="test",code="200"} 3`,
		`calendar_proxy_requests_total{feed="test",code="304"} 1`,
		`calendar_proxy_requests_total{feed="unknown",code="404"} 1`,
		`calendar_proxy_upstream_errors_total{feed="test"} 1`,
	} {
		if !strings.Contains(metrics.Body.String(), want) {
			t.Errorf("metrics lack %q:\n%s", want, metrics.Body.String())
		}
	}
	if strings.Contains(metrics.Body.String(), `upstream_errors_total{feed="unknown"}`) {
		t.Errorf("metrics report upstream errors for unknown tokens")
	}
}
