// share hosts the contents of uploaded zips at unguessable public URLs.
//
// It listens on two ports:
//   - admin (ADMIN_ADDR): upload, list and delete shares. Only reachable
//     through the internal ingress, behind tinyauth; it also requires
//     tinyauth's Remote-Email header to match ADMIN_EMAIL.
//   - public (PUBLIC_ADDR): serves /share/<secret>/... read-only. The
//     external ingress routes only to this port.
//
// Each share lives in DATA_DIR/<secret>/, holding meta.json and the
// extracted files/. Uploads are extracted into DATA_DIR/.tmp/ and renamed
// into place, so a share is never visible half-written.
package main

import (
	"archive/zip"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed admin.html
var templates embed.FS

var secretRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Expiry choices offered by the upload form. Zero means never.
var ttls = map[string]time.Duration{
	"1d":    24 * time.Hour,
	"7d":    7 * 24 * time.Hour,
	"30d":   30 * 24 * time.Hour,
	"never": 0,
}

type meta struct {
	Name    string     `json:"name"`
	Created time.Time  `json:"created"`
	Expires *time.Time `json:"expires,omitempty"`
}

func (m meta) expired(now time.Time) bool {
	return m.Expires != nil && now.After(*m.Expires)
}

type store struct {
	dir string
}

func (s store) tmpDir() string { return filepath.Join(s.dir, ".tmp") }

func (s store) meta(secret string) (meta, error) {
	var m meta
	b, err := os.ReadFile(filepath.Join(s.dir, secret, "meta.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// live returns the share's metadata, or an error if it doesn't exist or has
// expired.
func (s store) live(secret string) (meta, error) {
	if !secretRE.MatchString(secret) {
		return meta{}, fs.ErrNotExist
	}
	m, err := s.meta(secret)
	if err != nil {
		return m, err
	}
	if m.expired(time.Now()) {
		return m, fs.ErrNotExist
	}
	return m, nil
}

type share struct {
	Secret string
	meta
	Files int
	Bytes int64
}

func (s store) list() ([]share, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var shares []share
	for _, e := range entries {
		if !e.IsDir() || !secretRE.MatchString(e.Name()) {
			continue
		}
		m, err := s.meta(e.Name())
		if err != nil {
			slog.Warn("skipping share", "secret", e.Name(), "err", err)
			continue
		}
		sh := share{Secret: e.Name(), meta: m}
		filepath.WalkDir(filepath.Join(s.dir, e.Name(), "files"), func(_ string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					sh.Files++
					sh.Bytes += info.Size()
				}
			}
			return nil
		})
		shares = append(shares, sh)
	}
	slices.SortFunc(shares, func(a, b share) int { return b.Created.Compare(a.Created) })
	return shares, nil
}

func (s store) create(name string, ttl time.Duration, zipPath string, maxBytes int64) (string, error) {
	b := make([]byte, 16)
	rand.Read(b)
	secret := hex.EncodeToString(b)

	staging, err := os.MkdirTemp(s.tmpDir(), "share-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)

	if err := extract(zipPath, filepath.Join(staging, "files"), maxBytes); err != nil {
		return "", err
	}
	m := meta{Name: name, Created: time.Now().UTC()}
	if ttl > 0 {
		exp := m.Created.Add(ttl)
		m.Expires = &exp
	}
	mb, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(staging, "meta.json"), mb, 0o644); err != nil {
		return "", err
	}
	return secret, os.Rename(staging, filepath.Join(s.dir, secret))
}

func (s store) remove(secret string) error {
	if !secretRE.MatchString(secret) {
		return fs.ErrNotExist
	}
	return os.RemoveAll(filepath.Join(s.dir, secret))
}

func (s store) removeExpired() {
	shares, err := s.list()
	if err != nil {
		slog.Error("listing shares", "err", err)
		return
	}
	now := time.Now()
	for _, sh := range shares {
		if sh.expired(now) {
			slog.Info("removing expired share", "secret", sh.Secret, "name", sh.Name)
			if err := s.remove(sh.Secret); err != nil {
				slog.Error("removing share", "secret", sh.Secret, "err", err)
			}
		}
	}
}

// extract unpacks zipPath into dest, which must not exist. It writes only
// regular files and directories, stays inside dest (os.Root rejects
// escapes), and fails once the uncompressed total passes maxBytes.
//
// If every entry sits under one top-level directory, as when zipping a
// folder, that directory is stripped so its contents land at the share root.
// macOS "__MACOSX/" metadata is skipped.
func extract(zipPath, dest string, maxBytes int64) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("not a valid zip: %w", err)
	}
	defer zr.Close()

	type entry struct {
		f    *zip.File
		name string
	}
	var entries []entry
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if name == "__MACOSX/" || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		entries = append(entries, entry{f, name})
	}
	if prefix := commonDir(entries, func(e entry) string { return e.name }); prefix != "" {
		for i := range entries {
			entries[i].name = strings.TrimPrefix(entries[i].name, prefix)
		}
	}

	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()

	remaining := maxBytes
	for _, e := range entries {
		name := strings.TrimSuffix(e.name, "/")
		if name == "" {
			continue
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe path in zip: %q", e.f.Name)
		}
		mode := e.f.Mode()
		switch {
		case mode.IsDir():
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
			continue
		case !mode.IsRegular():
			continue // symlinks, devices
		}
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		n, err := extractFile(root, name, e.f, remaining)
		if err != nil {
			return err
		}
		remaining -= n
	}
	return nil
}

func extractFile(root *os.Root, name string, f *zip.File, limit int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if err != nil {
		return n, fmt.Errorf("extracting %q: %w", f.Name, err)
	}
	if n > limit {
		return n, errors.New("zip contents exceed the size limit")
	}
	return n, out.Close()
}

// commonDir returns "dir/" if every item's name starts with that same
// top-level directory, else "".
func commonDir[T any](items []T, name func(T) string) string {
	prefix := ""
	for _, it := range items {
		top, _, ok := strings.Cut(name(it), "/")
		if !ok {
			return "" // a file at the top level
		}
		if prefix == "" {
			prefix = top
		} else if top != prefix {
			return ""
		}
	}
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

// publicHandler serves /share/<secret>/<path>.
func publicHandler(s store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /share/{secret}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		secret := r.PathValue("secret")
		if _, err := s.live(secret); err != nil {
			http.NotFound(w, r)
			return
		}
		root, err := os.OpenRoot(filepath.Join(s.dir, secret, "files"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer root.Close()
		http.StripPrefix("/share/"+secret, http.FileServerFS(root.FS())).ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /share/{secret}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.PathValue("secret")+"/", http.StatusMovedPermanently)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Keep the secret out of Referer headers, search indexes and
		// Cloudflare's cache (which would outlive a delete).
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Robots-Tag", "noindex, nofollow")
		h.Set("Cache-Control", "private, no-store")
		mux.ServeHTTP(w, r)
	})
}

type admin struct {
	store      store
	email      string
	publicURL  string
	maxUpload  int64
	maxExtract int64
	page       *template.Template
}

func (a admin) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.index)
	mux.HandleFunc("POST /upload", a.upload)
	mux.HandleFunc("POST /delete/{secret}", a.delete)

	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Remote-Email"), a.email) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func (a admin) index(w http.ResponseWriter, r *http.Request) {
	shares, err := a.store.list()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.page.Execute(w, map[string]any{
		"Shares":    shares,
		"PublicURL": a.publicURL,
		"Created":   r.URL.Query().Get("created"),
		"Error":     r.URL.Query().Get("error"),
		"MaxUpload": a.maxUpload,
		"Now":       time.Now(),
	})
}

func (a admin) upload(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string) {
		http.Redirect(w, r, "/?error="+template.URLQueryEscaper(msg), http.StatusSeeOther)
	}

	r.Body = http.MaxBytesReader(w, r.Body, a.maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		fail(err.Error())
		return
	}
	tmp, err := os.CreateTemp(a.store.tmpDir(), "upload-*.zip")
	if err != nil {
		fail(err.Error())
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	var name, filename string
	ttl, gotFile := ttls["7d"], false
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail("upload failed: " + err.Error())
			return
		}
		switch p.FormName() {
		case "file":
			filename = p.FileName()
			if _, err := io.Copy(tmp, p); err != nil {
				fail("upload failed: " + err.Error())
				return
			}
			gotFile = true
		case "name":
			b, _ := io.ReadAll(io.LimitReader(p, 200))
			name = strings.TrimSpace(string(b))
		case "ttl":
			b, _ := io.ReadAll(io.LimitReader(p, 10))
			if d, ok := ttls[string(b)]; ok {
				ttl = d
			}
		}
	}
	if !gotFile {
		fail("no file uploaded")
		return
	}
	if err := tmp.Close(); err != nil {
		fail(err.Error())
		return
	}
	if name == "" {
		name = strings.TrimSuffix(filename, path.Ext(filename))
	}

	secret, err := a.store.create(name, ttl, tmp.Name(), a.maxExtract)
	if err != nil {
		slog.Error("creating share", "name", name, "err", err)
		fail(err.Error())
		return
	}
	slog.Info("created share", "secret", secret, "name", name, "ttl", ttl)
	http.Redirect(w, r, "/?created="+secret, http.StatusSeeOther)
}

func (a admin) delete(w http.ResponseWriter, r *http.Request) {
	secret := r.PathValue("secret")
	if err := a.store.remove(secret); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.Info("deleted share", "secret", secret)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBytes(key string, def int64) int64 {
	v, err := strconv.ParseInt(env(key, ""), 10, 64)
	if err != nil {
		return def
	}
	return v
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func main() {
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		slog.Error("ADMIN_EMAIL is required")
		os.Exit(1)
	}
	s := store{dir: env("DATA_DIR", "/data")}
	os.RemoveAll(s.tmpDir())
	if err := os.MkdirAll(s.tmpDir(), 0o755); err != nil {
		slog.Error("creating tmp dir", "err", err)
		os.Exit(1)
	}

	page := template.Must(template.New("admin.html").Funcs(template.FuncMap{
		"bytes": humanBytes,
	}).ParseFS(templates, "admin.html"))
	a := admin{
		store:      s,
		email:      email,
		publicURL:  strings.TrimSuffix(env("PUBLIC_URL", "https://home.ticklethepanda.co.uk/share"), "/"),
		maxUpload:  envBytes("MAX_UPLOAD_BYTES", 2<<30),
		maxExtract: envBytes("MAX_EXTRACT_BYTES", 4<<30),
		page:       page,
	}

	go func() {
		for {
			s.removeExpired()
			time.Sleep(10 * time.Minute)
		}
	}()

	errs := make(chan error, 2)
	serve := func(addr string, h http.Handler) {
		slog.Info("listening", "addr", addr)
		srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
		errs <- srv.ListenAndServe()
	}
	go serve(env("ADMIN_ADDR", ":8080"), a.handler())
	go serve(env("PUBLIC_ADDR", ":8081"), publicHandler(s))
	slog.Error("server stopped", "err", <-errs)
	os.Exit(1)
}
