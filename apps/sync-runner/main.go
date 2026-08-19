// sync-runner is a tiny HTTP wrapper around exactly one fixed sync command,
// selected at startup via RUNNER_MODE. It is not a general command executor:
// the HTTP API takes no parameters, and the set of possible jobs is fixed at
// compile time.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

const (
	maxBufferBytes  = 64 * 1024
	statusTailBytes = 4 * 1024
)

type jobFunc func(ctx context.Context, out *capBuffer) error

var jobs = map[string]jobFunc{
	"librofm": func(ctx context.Context, out *capBuffer) error {
		return runCmd(ctx, out, "librofm-download", "/audiobooks")
	},
	"libation": func(ctx context.Context, out *capBuffer) error {
		if err := runCmd(ctx, out, "/libation/LibationCli", "scan"); err != nil {
			// Mirrors liberate.sh's run(): don't liberate if scan failed.
			return err
		}
		return runCmd(ctx, out, "/libation/LibationCli", "liberate")
	},
}

// capBuffer is a mutex-guarded, size-capped byte buffer that command output
// is written into live, so /logs and /status reflect an in-progress run,
// not only a completed one.
type capBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (c *capBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > maxBufferBytes {
		c.buf = c.buf[len(c.buf)-maxBufferBytes:]
	}
	return len(p), nil
}

func (c *capBuffer) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, len(c.buf))
	copy(out, c.buf)
	return out
}

func (c *capBuffer) Tail(n int) []byte {
	b := c.Bytes()
	if len(b) > n {
		return b[len(b)-n:]
	}
	return b
}

// runCmd writes job output to both the capBuffer (served over /status and
// /logs) and the container's own stdout, so `kubectl logs` shows sync
// activity too, not just what the HTTP API happens to be asked for.
func runCmd(ctx context.Context, out *capBuffer, name string, args ...string) error {
	fmt.Fprintf(out, "$ %s %v\n", name, args)
	log.Printf("running: %s %v", name, args)
	mw := io.MultiWriter(out, os.Stdout)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = mw
	cmd.Stderr = mw
	return cmd.Run()
}

type lastResult struct {
	ExitCode int       `json:"exit_code"`
	Error    string    `json:"error,omitempty"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
}

var (
	mu      sync.Mutex
	running bool
	last    *lastResult
	output  = &capBuffer{}

	mode        string
	syncTimeout time.Duration
)

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	mu.Lock()
	if running {
		mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]string{"status": "already running"})
		return
	}
	running = true
	mu.Unlock()

	go execute()

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "started"})
}

func execute() {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	err := jobs[mode](ctx, output)

	result := &lastResult{
		ExitCode: exitCodeOf(err),
		Started:  started,
		Finished: time.Now(),
	}
	if err != nil {
		result.Error = err.Error()
	}

	mu.Lock()
	last = result
	running = false
	mu.Unlock()
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1 // e.g. context deadline exceeded, or failed to start at all
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	resp := map[string]any{"running": running}
	if last != nil {
		resp["last"] = last
	}
	mu.Unlock()
	resp["output_tail"] = string(output.Tail(statusTailBytes))
	writeJSON(w, resp)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(output.Bytes())
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// statusRecorder captures the response status so withAccessLog can report
// it -- http.ResponseWriter doesn't expose what was already written.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func withAccessLog(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	}
}

func main() {
	mode = os.Getenv("RUNNER_MODE")
	if _, ok := jobs[mode]; !ok {
		log.Fatalf("RUNNER_MODE must be one of the configured jobs, got %q", mode)
	}

	port := os.Getenv("RUNNER_PORT")
	if port == "" {
		port = "8080"
	}

	timeoutSeconds := 7200
	if v := os.Getenv("SYNC_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			timeoutSeconds = n
		}
	}
	syncTimeout = time.Duration(timeoutSeconds) * time.Second

	mux := http.NewServeMux()
	mux.HandleFunc("/run", withAccessLog(handleRun))
	mux.HandleFunc("/status", withAccessLog(handleStatus))
	mux.HandleFunc("/logs", withAccessLog(handleLogs))
	mux.HandleFunc("/healthz", withAccessLog(handleHealthz))

	addr := ":" + port
	log.Printf("sync-runner mode=%s listening on %s", mode, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
