package main

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
	return p
}

func TestExtractStripsCommonDir(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	z := writeZip(t, map[string]string{
		"handouts/a.pdf":        "a",
		"handouts/sub/b.pdf":    "b",
		"__MACOSX/handouts/._a": "junk",
	})
	if err := extract(z, dest, 1<<20); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.pdf", "sub/b.pdf"} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "__MACOSX")); err == nil {
		t.Error("__MACOSX was extracted")
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	z := writeZip(t, map[string]string{"ok.txt": "x", "../evil.txt": "x"})
	if err := extract(z, dest, 1<<20); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); err == nil {
		t.Fatal("file escaped dest")
	}
}

func TestExtractSizeLimit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	z := writeZip(t, map[string]string{"big.txt": strings.Repeat("x", 100)})
	if err := extract(z, dest, 50); err == nil {
		t.Fatal("expected size limit error")
	}
}

func TestPublicServing(t *testing.T) {
	s := store{dir: t.TempDir()}
	os.MkdirAll(s.tmpDir(), 0o755)
	z := writeZip(t, map[string]string{"index.html": "hello", "doc.txt": "doc"})
	secret, err := s.create("test", time.Hour, z, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	h := publicHandler(s)
	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec
	}

	if rec := get("/share/" + secret + "/doc.txt"); rec.Code != 200 || rec.Body.String() != "doc" {
		t.Errorf("doc.txt: %d %q", rec.Code, rec.Body)
	}
	if rec := get("/share/" + secret + "/"); rec.Code != 200 || rec.Body.String() != "hello" {
		t.Errorf("index: %d %q", rec.Code, rec.Body)
	}
	if rec := get("/share/" + secret); rec.Code != http.StatusMovedPermanently {
		t.Errorf("no slash: %d", rec.Code)
	}
	if rec := get("/share/" + strings.Repeat("0", 32) + "/doc.txt"); rec.Code != 404 {
		t.Errorf("unknown secret: %d", rec.Code)
	}
	if rec := get("/share/" + secret + "/../../" + secret + "/meta.json"); rec.Code == 200 {
		t.Errorf("meta.json reachable")
	}

	// Expire it.
	m, _ := s.meta(secret)
	past := time.Now().Add(-time.Minute)
	m.Expires = &past
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(s.dir, secret, "meta.json"), b, 0o644)
	if rec := get("/share/" + secret + "/doc.txt"); rec.Code != 404 {
		t.Errorf("expired: %d", rec.Code)
	}
	s.removeExpired()
	if _, err := os.Stat(filepath.Join(s.dir, secret)); err == nil {
		t.Error("expired share not removed")
	}
}

func TestAdminRequiresEmail(t *testing.T) {
	a := admin{store: store{dir: t.TempDir()}, email: "me@example.com"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Remote-Email", "someone@example.com")
	a.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d", rec.Code)
	}
}
