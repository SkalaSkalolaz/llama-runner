package download

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	// Retry tests must not sleep 2s/4s in real time.
	retryBaseDelay = 10 * time.Millisecond
}

// recordedRequest captures what the test server saw, so assertions can
// check HEAD-vs-GET behaviour, Range headers and auth propagation without
// exposing internals of the downloader.
type recordedRequest struct {
	method    string
	path      string
	rangeHdr  string
	auth      string
	userAgent string
}

// testServer is a configurable file server: files by path (missing path →
// 404), optional blanket gate status, optional failing-GET and
// abort-GET counters.
type testServer struct {
	mu sync.Mutex

	files      map[string][]byte
	gateStatus int // when non-zero: answer every request with this status
	headStatus int // when non-zero: answer HEAD with this status
	failGETs   int // answer this many GETs with 503 before serving
	abortAfter int // abort this many GETs mid-body after N bytes
	requests   []recordedRequest
}

func newTestServer(t *testing.T, files map[string][]byte) (*httptest.Server, *testServer) {
	ts := &testServer{files: files}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.requests = append(ts.requests, recordedRequest{
			method:    r.Method,
			path:      r.URL.Path,
			rangeHdr:  r.Header.Get("Range"),
			auth:      r.Header.Get("Authorization"),
			userAgent: r.Header.Get("User-Agent"),
		})
		if r.Method == http.MethodHead && ts.headStatus != 0 {
			st := ts.headStatus
			ts.mu.Unlock()
			w.WriteHeader(st)
			return
		}
		if ts.gateStatus != 0 {
			st := ts.gateStatus
			ts.mu.Unlock()
			w.WriteHeader(st)
			return
		}
		if r.Method == http.MethodGet && ts.failGETs > 0 {
			ts.failGETs--
			ts.mu.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		abort := ts.abortAfter
		if r.Method == http.MethodGet && abort > 0 {
			ts.abortAfter--
		}
		data, ok := ts.files[r.URL.Path]
		ts.mu.Unlock()

		if !ok {
			http.NotFound(w, r)
			return
		}
		start := rangeStart(r.Header.Get("Range"))
		if start > int64(len(data)) {
			start = int64(len(data))
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.WriteHeader(http.StatusOK)
			return
		}
		body := data[start:]
		if abort > 0 && len(body) > abort {
			// Announce the full remaining length, send only a chunk of
			// it, then break the connection — exactly the mid-transfer
			// death the client must notice (fewer bytes than declared)
			// and be able to resume from.
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			body = body[:abort]
			if start > 0 {
				w.WriteHeader(http.StatusPartialContent)
			}
			_, _ = w.Write(body)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			panic(http.ErrAbortHandler)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if start > 0 {
			w.WriteHeader(http.StatusPartialContent)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, ts
}

// rangeStart parses a "bytes=N-" Range header (0 when absent/foreign).
func rangeStart(h string) int64 {
	spec := strings.TrimPrefix(h, "bytes=")
	if i := strings.IndexByte(spec, '-'); i >= 0 {
		spec = spec[:i]
	}
	n, err := strconv.ParseInt(spec, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func (ts *testServer) count(method, path string) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	n := 0
	for _, r := range ts.requests {
		if r.method == method && r.path == path {
			n++
		}
	}
	return n
}

func (ts *testServer) all() []recordedRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]recordedRequest(nil), ts.requests...)
}

func TestFetch_SingleFile(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := bytes.Repeat([]byte("x"), 4096)
	srv, ts := newTestServer(t, map[string][]byte{"/model.gguf": payload})
	dir := t.TempDir()
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{
		Dir: dir, URL: srv.URL + "/model.gguf", Progress: &out, Version: "9.9.9", Token: "hf-super-secret",
	})
	if err != nil {
		t.Fatalf("Fetch: %v (out=%q)", err, out.String())
	}
	want := filepath.Join(dir, "model.gguf")
	got, rerr := os.ReadFile(want)
	if rerr != nil {
		t.Fatalf("downloaded file missing: %v", rerr)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("file content differs (%d vs %d bytes)", len(got), len(payload))
	}
	if res.EntryPoint != want || len(res.Files) != 1 || res.FromCache {
		t.Errorf("result = %+v, want EntryPoint=%s, 1 file, fresh", res, want)
	}
	if res.TotalBytes != int64(len(payload)) {
		t.Errorf("TotalBytes = %d, want %d", res.TotalBytes, len(payload))
	}
	for _, r := range ts.all() {
		if r.auth != "Bearer hf-super-secret" {
			t.Errorf("%s %s: Authorization = %q", r.method, r.path, r.auth)
		}
		if !strings.HasPrefix(r.userAgent, "llama-runner/9.9.9") {
			t.Errorf("%s %s: User-Agent = %q", r.method, r.path, r.userAgent)
		}
	}
	if strings.Contains(out.String(), "hf-super-secret") {
		t.Error("the token leaked into the progress output")
	}
}

func TestFetch_NonResolveURLStaysSingleFile(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := []byte("single-file-model")
	srv, ts := newTestServer(t, map[string][]byte{"/data/model-00001-of-00002.gguf": payload})
	dir := t.TempDir()
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/data/model-00001-of-00002.gguf", Progress: &out})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res.Files) != 1 {
		t.Errorf("Files = %v, want only the named file (no /resolve/ → no shard guessing)", res.Files)
	}
	if strings.Contains(out.String(), "sharded") {
		t.Errorf("non-HF URL must not be treated as sharded: %q", out.String())
	}
	// The sibling shard -00002-of-00002 must not even be requested.
	if n := ts.count("HEAD", "/data/model-00002-of-00002.gguf"); n != 0 {
		t.Errorf("sibling shard requested %d times", n)
	}
}

func TestFetch_MultiFile(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	srvName := func(i int) string {
		return "/org/repo/resolve/main/model-" + pad(i) + "-of-00003.gguf"
	}
	files := map[string][]byte{
		srvName(1): []byte("shard-one"),
		srvName(2): []byte("shard-two-longer"),
		srvName(3): []byte("shard3"),
	}
	srv, _ := newTestServer(t, files)
	dir := t.TempDir()
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + srvName(1), Progress: &out})
	if err != nil {
		t.Fatalf("Fetch: %v (out=%q)", err, out.String())
	}
	if len(res.Files) != 3 {
		t.Fatalf("Files = %v, want all 3 shards", res.Files)
	}
	for i := 1; i <= 3; i++ {
		want := filepath.Join(dir, "model-"+pad(i)+"-of-00003.gguf")
		if res.Files[i-1] != want {
			t.Errorf("Files[%d] = %q, want %q", i-1, res.Files[i-1], want)
		}
		if _, serr := os.Stat(want); serr != nil {
			t.Errorf("shard %d not on disk: %v", i, serr)
		}
	}
	if res.EntryPoint != res.Files[0] || !strings.HasSuffix(res.EntryPoint, "-00001-of-00003.gguf") {
		t.Errorf("EntryPoint = %q, want the first shard", res.EntryPoint)
	}
	if !strings.Contains(out.String(), "sharded model: 3 files") {
		t.Errorf("missing sharded notice: %q", out.String())
	}
	total := len("shard-one") + len("shard-two-longer") + len("shard3")
	if res.TotalBytes != int64(total) {
		t.Errorf("TotalBytes = %d, want %d", res.TotalBytes, total)
	}
}

// TestFetch_MultiFile_FromLastShard checks that passing any shard URL still
// downloads the whole set and points the entry at shard 1.
func TestFetch_MultiFile_FromLastShard(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	files := map[string][]byte{
		"/o/r/resolve/main/m-00001-of-00003.gguf": []byte("a"),
		"/o/r/resolve/main/m-00002-of-00003.gguf": []byte("b"),
		"/o/r/resolve/main/m-00003-of-00003.gguf": []byte("c"),
	}
	srv, _ := newTestServer(t, files)
	dir := t.TempDir()
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/o/r/resolve/main/m-00003-of-00003.gguf", Progress: &out, Verbose: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasSuffix(res.EntryPoint, "-00001-of-00003.gguf") {
		t.Errorf("EntryPoint = %q, want shard 1", res.EntryPoint)
	}
	if !strings.Contains(out.String(), "shard 3 was given") {
		t.Errorf("expected a warning that shard 3 was not the first: %q", out.String())
	}
}

func TestFetch_CachedSkipsGET(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := bytes.Repeat([]byte("S"), 2048)
	other := bytes.Repeat([]byte("T"), 2048)
	srv, ts := newTestServer(t, map[string][]byte{"/model.gguf": other})
	dir := t.TempDir()
	orig := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(orig, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf", Progress: &out, Verbose: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.FromCache {
		t.Error("FromCache = false, want true")
	}
	if got, _ := os.ReadFile(orig); !bytes.Equal(got, payload) {
		t.Error("cached file was modified although sizes matched")
	}
	if ts.count("HEAD", "/model.gguf") != 1 || ts.count("GET", "/model.gguf") != 0 {
		t.Errorf("expected exactly one HEAD and no GET, got %+v", ts.all())
	}
	if !strings.Contains(out.String(), "already downloaded") {
		t.Errorf("verbose cache notice missing: %q", out.String())
	}
}

func TestFetch_SizeMismatchRedownloads(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := bytes.Repeat([]byte("N"), 3000)
	srv, _ := newTestServer(t, map[string][]byte{"/model.gguf": payload})
	dir := t.TempDir()
	orig := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(orig, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf", Progress: &out})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.FromCache {
		t.Error("FromCache = true after a mismatching size")
	}
	if got, _ := os.ReadFile(orig); !bytes.Equal(got, payload) {
		t.Error("file was not replaced with the fresh content")
	}
	if !strings.Contains(out.String(), "size mismatch") {
		t.Errorf("mismatch notice missing: %q", out.String())
	}
}

func TestFetch_HeadNotFound(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	srv, ts := newTestServer(t, map[string][]byte{})
	dir := t.TempDir()

	_, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/missing.gguf"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a not-found error", err)
	}
	if ts.count("GET", "/missing.gguf") != 0 {
		t.Error("GET must not be attempted after a 404 HEAD")
	}
	if _, serr := os.Stat(filepath.Join(dir, "missing.gguf")); serr == nil {
		t.Error("no file must be created on failure")
	}
}

func TestFetch_HeadGated(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	srv, ts := newTestServer(t, map[string][]byte{"/private.gguf": []byte("data")})
	ts.gateStatus = http.StatusUnauthorized
	dir := t.TempDir()

	_, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/private.gguf"})
	if err == nil || !strings.Contains(err.Error(), "requires a token") {
		t.Fatalf("err = %v, want a gated-model error", err)
	}
	if len(ts.all()) != 1 {
		t.Errorf("expected only the HEAD request, got %+v", ts.all())
	}
}

// TestFetch_HeadUnknownProceeds: a server that rejects HEAD (405) must not
// block the download — the size check degrades to "unknown".
func TestFetch_HeadUnknownProceeds(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := []byte("payload")
	dir := t.TempDir()
	// A server variant that answers HEAD with 405 (method not allowed).
	srv2, ts := newTestServer(t, map[string][]byte{"/model.gguf": payload})
	ts.headStatus = http.StatusMethodNotAllowed

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv2.URL + "/model.gguf"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.TotalBytes != int64(len(payload)) {
		t.Errorf("TotalBytes = %d, want %d", res.TotalBytes, len(payload))
	}
}

func TestFetch_ResumeWithinCall(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := bytes.Repeat([]byte("R"), 1000)
	srv, ts := newTestServer(t, map[string][]byte{"/model.gguf": payload})
	ts.abortAfter = 1 // abort the first GET after a single byte
	dir := t.TempDir()
	var out bytes.Buffer

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf", Progress: &out})
	if err != nil {
		t.Fatalf("Fetch should have recovered via retry: %v (out=%q)", err, out.String())
	}
	got, rerr := os.ReadFile(filepath.Join(dir, "model.gguf"))
	if rerr != nil || !bytes.Equal(got, payload) {
		t.Errorf("resumed file wrong: %v (%d bytes)", rerr, len(got))
	}
	var sawRange bool
	for _, r := range ts.all() {
		if r.method == "GET" && r.rangeHdr != "" {
			sawRange = true
		}
	}
	if !sawRange {
		t.Error("the retry did not send a Range header")
	}
	if _, perr := os.Stat(filepath.Join(dir, "model.gguf"+partialSuffix)); perr == nil {
		t.Error("the partial must be renamed away on success")
	}
	if res.FromCache {
		t.Error("a resumed download is not a cache hit")
	}
}

// TestFetch_PartialFileSurvivesFailedRun emulates a run that dies mid-download
// (every GET aborts) followed by a second run: the .partial left behind
// must be resumed with Range, not restarted.
//
// The .partial file surviving a failed Fetch is what makes the next run
// (a new process in production) able to resume — the same mechanism as
// within one call, no separate cross-process code path exists.
func TestFetch_PartialFileSurvivesFailedRun(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := bytes.Repeat([]byte("P"), 800)
	srv, ts := newTestServer(t, map[string][]byte{"/model.gguf": payload})
	ts.abortAfter = 100 // every GET aborts after 100 bytes
	dir := t.TempDir()

	if _, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf"}); err == nil {
		t.Fatal("the first Fetch must fail while the server keeps aborting")
	}
	part := filepath.Join(dir, "model.gguf"+partialSuffix)
	info, serr := os.Stat(part)
	if serr != nil {
		t.Fatalf("partial file must survive the failed fetch: %v", serr)
	}
	firstPartial := info.Size()

	// The server heals; a fresh Fetch (new process in real life) resumes.
	ts.mu.Lock()
	ts.abortAfter = 0
	ts.mu.Unlock()

	res, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf"})
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	got, rerr := os.ReadFile(filepath.Join(dir, "model.gguf"))
	if rerr != nil || !bytes.Equal(got, payload) {
		t.Fatalf("resumed content wrong: %v (%d bytes)", rerr, len(got))
	}
	var sawRangeAfter bool
	for _, r := range ts.all() {
		if r.method == "GET" && rangeStart(r.rangeHdr) >= firstPartial && r.rangeHdr != "" {
			sawRangeAfter = true
		}
	}
	if !sawRangeAfter {
		t.Error("the second Fetch did not resume with Range from the partial size")
	}
	if res.TotalBytes != int64(len(payload)) {
		t.Errorf("TotalBytes = %d, want full size %d", res.TotalBytes, len(payload))
	}
}

func TestFetch_RetryOn5xxThenSuccess(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := []byte("after-retries")
	srv, ts := newTestServer(t, map[string][]byte{"/model.gguf": payload})
	ts.failGETs = 2 // two 503s, then normal service
	dir := t.TempDir()
	var out bytes.Buffer

	if _, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf", Progress: &out}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if n := ts.count("GET", "/model.gguf"); n != 3 {
		t.Errorf("GET count = %d, want 3 (two 503s + success)", n)
	}
	if !strings.Contains(out.String(), "retrying") {
		t.Errorf("retry notice missing: %q", out.String())
	}
}

func TestFetch_RetryExhaustedFails(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	srv, ts := newTestServer(t, map[string][]byte{"/model.gguf": []byte("x")})
	ts.failGETs = 99 // never heals
	dir := t.TempDir()

	_, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf"})
	if err == nil {
		t.Fatal("Fetch must fail when all retries return 503")
	}
	if n := ts.count("GET", "/model.gguf"); n != retryAttempts {
		t.Errorf("GET count = %d, want %d", n, retryAttempts)
	}
}

func TestFetch_CreatesMissingDir(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	srv, _ := newTestServer(t, map[string][]byte{"/model.gguf": []byte("abc")})
	dir := filepath.Join(t.TempDir(), "new", "nested")

	if _, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	info, serr := os.Stat(dir)
	if serr != nil || !info.IsDir() {
		t.Fatalf("directory not created: %v", serr)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %o, want 700", perm)
	}
}

func TestFetch_NilProgressIsSilent(t *testing.T) {
	srv, _ := newTestServer(t, map[string][]byte{"/model.gguf": []byte("abc")})
	dir := t.TempDir()
	if _, err := Fetch(context.Background(), Options{Dir: dir, URL: srv.URL + "/model.gguf", Progress: nil}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
}

func TestFetch_RejectsBadURL(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	if _, err := Fetch(context.Background(), Options{Dir: t.TempDir(), URL: "not-a-url"}); err == nil {
		t.Error("a malformed URL must be rejected")
	}
}

func TestFetch_ShardNotFirst_QuietWithoutVerbose(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	files := map[string][]byte{
		"/o/r/resolve/main/m-00001-of-00002.gguf": []byte("a"),
		"/o/r/resolve/main/m-00002-of-00002.gguf": []byte("b"),
	}
	srv, _ := newTestServer(t, files)
	var out bytes.Buffer
	res, err := Fetch(context.Background(), Options{
		Dir:      t.TempDir(),
		URL:      srv.URL + "/o/r/resolve/main/m-00002-of-00002.gguf",
		Progress: &out,
		Verbose:  false,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.HasSuffix(res.EntryPoint, "-00001-of-00002.gguf") {
		t.Errorf("EntryPoint = %q, want shard 1", res.EntryPoint)
	}
	if strings.Contains(out.String(), "shard 2 was given") {
		t.Errorf("shard-not-first warning must be quiet without -v: %q", out.String())
	}
}

// pad renders a 1-based shard index as the 5-digit HF width (00001…).
func pad(i int) string { return fmt.Sprintf("%05d", i) }
