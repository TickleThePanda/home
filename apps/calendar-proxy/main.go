// calendar-proxy serves filtered copies of iCalendar feeds, small enough
// for Proton Calendar's 1 MB subscription limit.
//
// Each feed in CONFIG_FILE has a secret token and serves at
// /calendar/<token>.ics. The proxy fetches the source on request, filters
// it (see Filter), and caches the result for cacheTTL. If the source fails,
// it serves the last good copy.
//
// Tokens and source URLs are secrets: they never appear in logs or metrics.
// Feeds are identified there by name.
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // the image is FROM scratch

	// Also for scratch: CA roots for fetching HTTPS sources.
	_ "golang.org/x/crypto/x509roots/fallback"
	"gopkg.in/yaml.v3"
)

const (
	cacheTTL       = 5 * time.Minute
	maxSourceBytes = 64 << 20
)

type Config struct {
	Defaults FeedSettings `yaml:"defaults"`
	Feeds    []Feed       `yaml:"feeds"`
}

type FeedSettings struct {
	PastDays *int `yaml:"past_days"`
	MaxBytes *int `yaml:"max_bytes"`
}

type Feed struct {
	FeedSettings `yaml:",inline"`
	// Name labels the feed in logs and metrics.
	Name   string `yaml:"name"`
	Token  string `yaml:"token"`
	Source string `yaml:"source"`
	Self   string `yaml:"self"`
}

var builtinDefaults = struct{ PastDays, MaxBytes int }{90, 900_000}

func loadConfig(path string) (*Config, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	for i := range c.Feeds {
		f := &c.Feeds[i]
		if f.Name == "" {
			f.Name = fmt.Sprintf("feed%d", i)
		}
		if len(f.Token) < 16 || f.Source == "" {
			return nil, nil, fmt.Errorf("feed %s: needs a token of 16+ characters and a source", f.Name)
		}
	}
	return &c, raw, nil
}

func (c *Config) settings(f *Feed) (pastDays, maxBytes int) {
	pastDays, maxBytes = builtinDefaults.PastDays, builtinDefaults.MaxBytes
	for _, s := range []FeedSettings{c.Defaults, f.FeedSettings} {
		if s.PastDays != nil {
			pastDays = *s.PastDays
		}
		if s.MaxBytes != nil {
			maxBytes = *s.MaxBytes
		}
	}
	return pastDays, maxBytes
}

type cached struct {
	body    []byte
	etag    string
	fetched time.Time
	stats   Stats
}

type server struct {
	configPath string
	client     *http.Client
	now        func() time.Time

	mu      sync.Mutex
	feedMu  map[string]*sync.Mutex // per feed name: one fetch at a time
	cache   map[string]*cached     // key: feed name + config hash
	errors  map[string]int         // upstream failures per feed name
	lastHit map[string]*cached     // latest result per feed name, for metrics
	// requests counts feed requests by feed name ("unknown" for a bad
	// token) and status code.
	requests map[string]map[int]int
}

// Cache states for the request log: served from cache, refreshed from the
// source, or the source failed and an older copy was served.
const (
	cacheHit   = "hit"
	cacheMiss  = "miss"
	cacheStale = "stale"
)

func newServer(configPath string) *server {
	return &server{
		configPath: configPath,
		client:     &http.Client{Timeout: 60 * time.Second},
		now:        time.Now,
		feedMu:     map[string]*sync.Mutex{},
		cache:      map[string]*cached{},
		errors:     map[string]int{},
		lastHit:    map[string]*cached{},
		requests:   map[string]map[int]int{},
	}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /calendar/{file}", s.serveFeed)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok\n") })
	mux.HandleFunc("GET /metrics", s.serveMetrics)
	return mux
}

func main() {
	s := newServer(envOr("CONFIG_FILE", "/config/config.yaml"))
	cfg, _, err := loadConfig(s.configPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	for i := range cfg.Feeds {
		f := &cfg.Feeds[i]
		pastDays, maxBytes := cfg.settings(f)
		slog.Info("feed configured", "feed", f.Name, "past_days", pastDays, "max_bytes", maxBytes, "self_set", f.Self != "")
	}
	addr := envOr("ADDR", ":8080")
	slog.Info("listening", "addr", addr)
	srv := &http.Server{Addr: addr, Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// serveFeed serves /calendar/<token>.ics and logs one line per request.
// The log names the feed, never the token.
func (s *server) serveFeed(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	feedName, state, status, size := "unknown", "", http.StatusOK, 0
	defer func() {
		attrs := []any{"feed", feedName, "status", status, "bytes", size,
			"duration", time.Since(start).Round(time.Millisecond),
			"client", clientIP(r), "user_agent", r.UserAgent()}
		if state != "" {
			attrs = append(attrs, "cache", state)
		}
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "request", attrs...)
		s.mu.Lock()
		if s.requests[feedName] == nil {
			s.requests[feedName] = map[int]int{}
		}
		s.requests[feedName][status]++
		s.mu.Unlock()
	}()

	token, ok := strings.CutSuffix(r.PathValue("file"), ".ics")
	if !ok {
		status = http.StatusNotFound
		http.NotFound(w, r)
		return
	}
	// Read per request: a mounted Secret updates in place, and requests
	// are rare (Proton polls a few times an hour).
	cfg, raw, err := loadConfig(s.configPath)
	if err != nil {
		slog.Error("config", "err", err)
		status = http.StatusInternalServerError
		http.Error(w, "config error", status)
		return
	}
	var feed *Feed
	for i := range cfg.Feeds {
		if subtle.ConstantTimeCompare([]byte(cfg.Feeds[i].Token), []byte(token)) == 1 {
			feed = &cfg.Feeds[i]
		}
	}
	if feed == nil {
		status = http.StatusNotFound
		http.NotFound(w, r)
		return
	}
	feedName = feed.Name

	var c *cached
	c, state, err = s.get(cfg, feed, raw)
	if err != nil {
		slog.Error("feed unavailable", "feed", feed.Name, "err", err)
		status = http.StatusBadGateway
		http.Error(w, "upstream calendar unavailable", status)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("ETag", c.etag)
	if r.Header.Get("If-None-Match") == c.etag {
		status = http.StatusNotModified
		w.WriteHeader(status)
		return
	}
	size = len(c.body)
	w.Write(c.body)
}

// clientIP is the caller's address as Cloudflare saw it, falling back to
// the proxy chain and then the direct peer.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(ip)
	}
	return r.RemoteAddr
}

// get returns the feed's cached result and where it came from (cacheHit,
// cacheMiss or cacheStale), refreshing it when older than cacheTTL. A
// refresh failure falls back to the stale copy.
func (s *server) get(cfg *Config, feed *Feed, rawConfig []byte) (*cached, string, error) {
	sum := sha256.Sum256(rawConfig)
	key := feed.Name + "/" + hex.EncodeToString(sum[:8])

	s.mu.Lock()
	fm := s.feedMu[feed.Name]
	if fm == nil {
		fm = &sync.Mutex{}
		s.feedMu[feed.Name] = fm
	}
	s.mu.Unlock()
	fm.Lock()
	defer fm.Unlock()

	s.mu.Lock()
	c := s.cache[key]
	s.mu.Unlock()
	if c != nil && s.now().Sub(c.fetched) < cacheTTL {
		return c, cacheHit, nil
	}

	s.mu.Lock()
	prev := s.lastHit[feed.Name]
	s.mu.Unlock()
	fresh, err := s.refresh(cfg, feed, prev)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.errors[feed.Name]++
		if c == nil {
			c = s.lastHit[feed.Name] // config changed, but old output beats none
		}
		if c != nil {
			slog.Warn("refresh failed, serving stale copy", "feed", feed.Name, "age", s.now().Sub(c.fetched).Round(time.Second), "err", err)
			return c, cacheStale, nil
		}
		return nil, "", err
	}
	for k := range s.cache {
		if strings.HasPrefix(k, feed.Name+"/") {
			delete(s.cache, k)
		}
	}
	s.cache[key] = fresh
	s.lastHit[feed.Name] = fresh
	return fresh, cacheMiss, nil
}

// refresh fetches and filters the feed. prev, the last good result if any,
// only feeds the log's "changed" flag.
func (s *server) refresh(cfg *Config, feed *Feed, prev *cached) (*cached, error) {
	fetchStart := time.Now()
	src, err := s.fetch(feed.Source)
	fetchTime := time.Since(fetchStart).Round(time.Millisecond)
	if err != nil {
		return nil, err
	}
	pastDays, maxBytes := cfg.settings(feed)
	now := s.now()
	out, st, err := Filter(src, Options{Now: now, PastDays: pastDays, MaxBytes: maxBytes, Self: feed.Self})
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	sum := sha256.Sum256(out)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	attrs := []any{"feed", feed.Name, "fetch", fetchTime, "in_bytes", st.InBytes, "out_bytes", st.OutBytes,
		"kept", st.Kept, "dropped", st.Dropped, "past_days", st.PastDays,
		"changed", prev == nil || prev.etag != etag}
	switch {
	case st.OutBytes > maxBytes:
		slog.Error("feed still over size limit", append(attrs, "max_bytes", maxBytes)...)
	case len(st.Degraded) > 0:
		slog.Warn("feed shrunk to fit", append(attrs, "degraded", st.Degraded)...)
	default:
		slog.Info("feed refreshed", attrs...)
	}
	return &cached{body: out, etag: etag, fetched: now, stats: st}, nil
}

// fetch GETs the source. Errors never include the URL, which is secret.
func (s *server) fetch(source string) ([]byte, error) {
	resp, err := s.client.Get(source)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch: read: %w", err)
	}
	if len(body) > maxSourceBytes {
		return nil, fmt.Errorf("fetch: source over %d bytes", maxSourceBytes)
	}
	return body, nil
}

func (s *server) serveMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	names := make([]string, 0, len(s.lastHit))
	for n := range s.lastHit {
		names = append(names, n)
	}
	for n := range s.errors {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	for n := range s.requests {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)

	metric := func(name, typ, help string, each func(feed string)) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
		for _, n := range names {
			each(n)
		}
	}
	metric("calendar_proxy_requests_total", "counter", "Feed requests, by status code.", func(n string) {
		codes := make([]int, 0, len(s.requests[n]))
		for c := range s.requests[n] {
			codes = append(codes, c)
		}
		slices.Sort(codes)
		for _, c := range codes {
			fmt.Fprintf(w, "calendar_proxy_requests_total{feed=%q,code=\"%d\"} %d\n", n, c, s.requests[n][c])
		}
	})
	metric("calendar_proxy_upstream_errors_total", "counter", "Failed source fetches or parses.", func(n string) {
		if _, ok := s.errors[n]; ok || s.lastHit[n] != nil {
			fmt.Fprintf(w, "calendar_proxy_upstream_errors_total{feed=%q} %d\n", n, s.errors[n])
		}
	})
	stat := func(n string) (Stats, bool) {
		if c := s.lastHit[n]; c != nil {
			return c.stats, true
		}
		return Stats{}, false
	}
	metric("calendar_proxy_source_bytes", "gauge", "Size of the last fetched source feed.", func(n string) {
		if st, ok := stat(n); ok {
			fmt.Fprintf(w, "calendar_proxy_source_bytes{feed=%q} %d\n", n, st.InBytes)
		}
	})
	metric("calendar_proxy_output_bytes", "gauge", "Size of the last filtered feed.", func(n string) {
		if st, ok := stat(n); ok {
			fmt.Fprintf(w, "calendar_proxy_output_bytes{feed=%q} %d\n", n, st.OutBytes)
		}
	})
	metric("calendar_proxy_events_kept", "gauge", "Events in the last filtered feed.", func(n string) {
		if st, ok := stat(n); ok {
			fmt.Fprintf(w, "calendar_proxy_events_kept{feed=%q} %d\n", n, st.Kept)
		}
	})
	metric("calendar_proxy_events_dropped", "gauge", "Events dropped from the last filtered feed, by reason.", func(n string) {
		if st, ok := stat(n); ok {
			reasons := make([]string, 0, len(st.Dropped))
			for r := range st.Dropped {
				reasons = append(reasons, r)
			}
			slices.Sort(reasons)
			for _, r := range reasons {
				fmt.Fprintf(w, "calendar_proxy_events_dropped{feed=%q,reason=%q} %d\n", n, r, st.Dropped[r])
			}
		}
	})
	metric("calendar_proxy_past_days", "gauge", "Past window used for the last filtered feed, after shrinking for size.", func(n string) {
		if st, ok := stat(n); ok {
			fmt.Fprintf(w, "calendar_proxy_past_days{feed=%q} %d\n", n, st.PastDays)
		}
	})
	metric("calendar_proxy_degraded", "gauge", "1 if the last filtered feed needed size degradation.", func(n string) {
		if st, ok := stat(n); ok {
			v := 0
			if len(st.Degraded) > 0 {
				v = 1
			}
			fmt.Fprintf(w, "calendar_proxy_degraded{feed=%q} %d\n", n, v)
		}
	})
}
