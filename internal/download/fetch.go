// Package download implements the -download one-shot pre-step: fetching a
// GGUF model (single-file or Hugging Face multi-file/sharded) over HTTP into
// a local directory, so the launcher can continue with the freshly stored
// file as if the user had always pointed -model at it.
//
// The package is deliberately independent of internal/app and
// internal/proxy: it takes a context, an Options struct and a progress
// writer, and returns a Result. Its only project dependencies are config
// (for ExpandHome) and i18n (for user-facing messages).
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

const (
	// partialSuffix marks an in-flight download file. A finished transfer
	// is renamed onto the target, so a target file always is complete and
	// an interrupted one leaves a resumable .partial behind.
	partialSuffix = ".partial"

	// dirMode is the permission for the destination directory; model
	// directories are private by default (the user's GGUF dir may hold
	// gated downloads).
	dirMode = 0o700

	// headerTimeout bounds only the wait for response headers — the body of
	// a multi-gigabyte download must never be cut by a client Timeout.
	headerTimeout = 60 * time.Second

	// retryAttempts: transient (transport and 5xx) failures are retried
	// with exponential backoff (retryBaseDelay · 2^attempt).
	retryAttempts = 3
)

// retryBaseDelay is a var so tests can shrink the backoff sleep;
// production never changes it from 2 seconds (delays: 2s, 4s).
var retryBaseDelay = 2 * time.Second

// Options configures one Fetch call.
type Options struct {
	// Dir is the absolute destination directory (leading "~" already
	// expanded — see ParseSpec). Created if missing.
	Dir string
	// URL is the model URL exactly as the user provided it.
	URL string
	// Progress receives the progress and diagnostic lines; nil disables
	// all output.
	Progress io.Writer
	// Verbose enables per-file cache/diagnostic lines (the -v switch).
	Verbose bool
	// Version is the launcher version, embedded in the User-Agent.
	Version string
	// Token is the Hugging Face access token (HF_TOKEN /
	// HUGGING_FACE_HUB_TOKEN). It is only ever sent as a bearer header and
	// must never appear in any output.
	Token string
}

// Result reports what Fetch placed on disk.
type Result struct {
	// Files lists the absolute paths of all files of the model, in shard
	// order (a single-file model has exactly one entry).
	Files []string
	// EntryPoint is the path to pass to llama-server: the -00001-of-NNNNN
	// shard for a multi-file model, the only file otherwise.
	EntryPoint string
	// TotalBytes is the size of all files of the model (cache hits
	// included).
	TotalBytes int64
	// FromCache is true when every file was already present at the
	// expected size and nothing was transferred.
	FromCache bool
}

// httpStatusError carries an unexpected HTTP status out of the transport
// layer so the user-facing message (gated / not found / generic) is chosen
// once, at the top of downloadOne, instead of being wrapped twice on the way
// up.
type httpStatusError struct {
	code int
	url  string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d on %s", e.code, e.url)
}

// Fetch downloads the model behind opts.URL into opts.Dir and returns the
// resulting file set. A Hugging Face resolve-URL whose file name follows the
// sharded scheme (…-00001-of-00004.gguf, total > 1) expands to the whole
// shard set; any other URL downloads the single file it points at.
//
// Per file the rules are: an existing target with a size matching HEAD is
// skipped; an existing target with a mismatched (or HEAD-less) size is
// re-downloaded; a leftover <target>.partial is resumed with a Range
// request. Transfers go to <target>.partial and are renamed onto the target
// atomically, so an interrupted run never leaves a half-written "complete"
// file behind. Transient failures (transport errors, HTTP 5xx) are retried
// with exponential backoff; 401/403 and 404 are reported immediately with
// dedicated messages.
//
// ctx bounds the whole operation, including the backoff sleeps.
func Fetch(ctx context.Context, opts Options) (Result, error) {
	u, err := url.Parse(opts.URL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return Result{}, i18n.Errorv(i18n.MsgErrDownloadSpec, opts.URL)
	}
	fileName := path.Base(u.Path)
	if fileName == "" || fileName == "." || fileName == "/" {
		return Result{}, i18n.Errorv(i18n.MsgErrDownloadSpec, opts.URL)
	}

	// Shard expansion applies only to Hugging Face style resolve-URLs:
	// the -NNNNN-of-MMMMM scheme is an HF convention and guessing it on
	// arbitrary URLs would fabricate paths that do not exist.
	names := []string{fileName}
	if isHFResolveURL(u) {
		if sp, ok := DetectShards(fileName); ok && sp.MultiFile() {
			if opts.Progress != nil {
				fmt.Fprint(opts.Progress, i18n.TF(i18n.MsgDownloadShards, sp.Total))
				if sp.Cur != 1 && opts.Verbose {
					fmt.Fprint(opts.Progress, i18n.TF(i18n.MsgWarnDownloadShardNotFirst, sp.Cur))
				}
			}
			names = make([]string, sp.Total)
			for i := range names {
				names[i] = sp.Name(i + 1)
			}
		}
	}

	if err := os.MkdirAll(opts.Dir, dirMode); err != nil {
		return Result{}, i18n.Errorf(i18n.MsgErrDownloadDisk, err, opts.Dir)
	}

	client := newClient(opts)
	res := Result{FromCache: true}
	for _, name := range names {
		fileURL := opts.URL
		if name != fileName {
			fileURL, err = ShardURL(opts.URL, name)
			if err != nil {
				return Result{}, i18n.Errorf(i18n.MsgErrDownloadSpec, err, name)
			}
		}
		target := JoinDir(opts.Dir, name)
		if opts.Progress != nil {
			fmt.Fprint(opts.Progress, i18n.TF(i18n.MsgDownloadStarting, fileURL, target))
		}
		size, cached, derr := downloadOne(ctx, client, fileURL, target, opts)
		if derr != nil {
			return Result{}, derr
		}
		res.Files = append(res.Files, target)
		res.TotalBytes += size
		if !cached {
			res.FromCache = false
		}
	}
	// Shard 1 is always the first entry of names, which is the file
	// llama-server wants as -m.
	res.EntryPoint = res.Files[0]
	return res, nil
}

// isHFResolveURL reports whether u has the Hugging Face model-file shape
// (a /resolve/<revision>/ path) — the only shape whose sharded names may
// be expanded. The host is deliberately NOT required to be
// huggingface.co itself: mirrors (hf-mirror.com and friends) serve the
// identical /resolve/ layout, and the check is what keeps arbitrary URLs
// from having shard siblings fabricated. It also lets the downloader be
// exercised against a local test server.
func isHFResolveURL(u *url.URL) bool {
	return strings.Contains(u.Path, "/resolve/")
}

// newClient builds the HTTP client for the transfer: no whole-request
// Timeout (gigabyte bodies must not be cut), a bounded header wait, and a
// round tripper stamping the User-Agent and optional bearer token. Redirects
// (HF answers 302 towards its CDN) follow the http package default of up to
// 10 hops.
func newClient(opts Options) *http.Client {
	ua := "llama-runner (auto-download)"
	if opts.Version != "" {
		ua = fmt.Sprintf("llama-runner/%s (auto-download)", opts.Version)
	}
	return &http.Client{
		Timeout: 0,
		Transport: &headerRoundTripper{
			base: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				ResponseHeaderTimeout: headerTimeout,
			},
			token:     opts.Token,
			userAgent: ua,
		},
	}
}

// headerRoundTripper stamps every outgoing request with the User-Agent and,
// when configured, the Hugging Face bearer token. The token exists only
// here — it is never formatted into a message or an error.
type headerRoundTripper struct {
	base      http.RoundTripper
	token     string
	userAgent string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

// downloadOne transfers (or reuses) one file and returns its size on disk
// and whether it came from the local cache. fileURL is the URL of exactly
// this file; target is the absolute destination path.
func downloadOne(ctx context.Context, client *http.Client, fileURL, target string, opts Options) (int64, bool, error) {
	name := filepath.Base(target)

	remoteSize, haveRemote, herr := remoteSize(ctx, client, fileURL)
	if herr != nil {
		return 0, false, userError(herr, fileURL) // gated / not-found / unreachable
	}
	if local, lerr := os.Stat(target); lerr == nil {
		if haveRemote && local.Size() == remoteSize {
			if opts.Verbose && opts.Progress != nil {
				fmt.Fprint(opts.Progress, i18n.TF(i18n.MsgDownloadFromCache, target))
			}
			return local.Size(), true, nil
		}
		if haveRemote && opts.Progress != nil {
			fmt.Fprint(opts.Progress, i18n.TF(i18n.MsgDownloadSizeMismatch, local.Size(), remoteSize))
		}
		// Size differs or the server gave nothing to compare against:
		// re-download and overwrite through the partial + rename path.
	}

	part := target + partialSuffix
	hadPartial := partialSize(part) > 0

	pg := newProgressWriter(opts.Progress, name, expectedSize(remoteSize, haveRemote), time.Now())
	size, terr := transferWithRetry(ctx, client, fileURL, part, pg)
	if terr != nil {
		pg.Done(0, hadPartial, terr)
		return 0, false, userError(terr, fileURL)
	}
	if rerr := os.Rename(part, target); rerr != nil {
		pg.Done(0, hadPartial, rerr)
		return 0, false, i18n.Errorf(i18n.MsgErrDownloadFailed, rerr, target)
	}
	pg.Done(size, false, nil)
	if opts.Verbose && opts.Progress != nil {
		fmt.Fprint(opts.Progress, i18n.TF(i18n.MsgDownloadURLPath, fileURL, target))
	}
	return size, false, nil
}

// userError turns a transport-layer failure into the message the user sees:
// HTTP statuses get their dedicated localization (gated, not found,
// generic), anything else (transport, filesystem) is wrapped into
// MsgErrDownloadFailed with the URL for context.
func userError(err error, fileURL string) error {
	var hse *httpStatusError
	if errors.As(err, &hse) {
		switch hse.code {
		case http.StatusUnauthorized, http.StatusForbidden:
			return i18n.Errorv(i18n.MsgDownloadGated, hse.url)
		case http.StatusNotFound:
			return i18n.Errorv(i18n.MsgDownloadNotFound, hse.url)
		default:
			return i18n.Errorv(i18n.MsgDownloadHTTPError, hse.code, hse.url)
		}
	}
	return i18n.Errorf(i18n.MsgErrDownloadFailed, err, fileURL)
}

// remoteSize issues a HEAD request to learn the remote file size.
//
// Hard errors (returned to the caller): 401/403 and 404 as *httpStatusError,
// any transport failure as the raw error — no download can be expected to
// work when even HEAD cannot reach the server. Everything else (other status
// codes, missing Content-Length) degrades to "size unknown": the download
// proceeds and the server's GET response makes the final decision.
func remoteSize(ctx context.Context, client *http.Client, fileURL string) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer drainClose(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return 0, false, &httpStatusError{code: resp.StatusCode, url: fileURL}
	case http.StatusOK, http.StatusPartialContent:
		if resp.ContentLength > 0 {
			return resp.ContentLength, true, nil
		}
		return 0, false, nil
	default:
		return 0, false, nil // unknown answer: proceed, GET decides
	}
}

// transferWithRetry runs the GET-and-write transfer, retrying transient
// failures (transport errors, HTTP 5xx) with exponential backoff
// (retryBaseDelay · 2^attempt) up to retryAttempts times. The partial file
// is kept between attempts and its current size is re-read before every
// one, so each retry resumes exactly where the previous attempt stopped.
// Deterministic outcomes (success, 4xx) return immediately.
func transferWithRetry(ctx context.Context, client *http.Client, fileURL, part string, pg *progressWriter) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay << (attempt - 1)
			if pg.out != nil {
				fmt.Fprint(pg.out, i18n.TF(i18n.MsgDownloadRetry, delay, attempt+1, retryAttempts, lastErr))
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return 0, ctx.Err()
			case <-timer.C:
			}
		}
		size, done, err := transferOnce(ctx, client, fileURL, part, partialSize(part), pg)
		if done {
			return size, err
		}
		lastErr = err
	}
	return 0, lastErr
}

// transferOnce performs one GET attempt and streams the body into part
// starting at offset bytes. The done flag tells the retry loop the outcome
// is final (success or a deterministic HTTP error); done=false with an error
// means a transient failure worth retrying. A resumed attempt (offset > 0)
// sends a Range header; a server answering 200 instead of 206 ignored it, so
// the partial is truncated and the file starts over.
func transferOnce(ctx context.Context, client *http.Client, fileURL, part string, offset int64, pg *progressWriter) (size int64, done bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return 0, true, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, err // transport-level: retry
	}
	defer drainClose(resp.Body)

	resumed := false
	switch resp.StatusCode {
	case http.StatusPartialContent:
		resumed = offset > 0
	case http.StatusOK:
		// Range ignored (or a fresh start): overwrite from zero.
		offset = 0
	case http.StatusRequestedRangeNotSatisfiable:
		// The stored partial no longer matches the remote file (it changed
		// server-side, or the offset is past EOF): drop it and fail this
		// attempt deterministically — the caller retries from scratch on
		// the next run.
		if rmErr := os.Remove(part); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return 0, true, rmErr
		}
		return 0, true, errors.New("HTTP 416: stored partial does not match the remote file")
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return 0, true, &httpStatusError{code: resp.StatusCode, url: fileURL}
	default:
		statusErr := &httpStatusError{code: resp.StatusCode, url: fileURL}
		if resp.StatusCode >= 500 {
			return 0, false, statusErr // server-side: retry
		}
		return 0, true, statusErr
	}

	flags := os.O_CREATE | os.O_WRONLY
	if resumed {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, ferr := os.OpenFile(part, flags, 0o600)
	if ferr != nil {
		return 0, true, ferr
	}
	// The progress writer counts bytes and renders the throttled line;
	// MultiWriter tees the body into the file and through it.
	written, cerr := io.Copy(io.MultiWriter(f, pg), resp.Body)
	// Keep whatever was flushed even on a broken stream: the partial must
	// reflect exactly the bytes on disk for the next resume.
	ferr = f.Close()
	if cerr != nil {
		return 0, false, cerr // interrupted stream: retry resumes from partial
	}
	if ferr != nil {
		return 0, true, ferr
	}
	return offset + written, true, nil
}

// partialSize returns the size of an existing partial file, or 0 when there
// is none (or it cannot be stat'ed — a fresh start is always safe).
func partialSize(part string) int64 {
	if info, err := os.Stat(part); err == nil {
		return info.Size()
	}
	return 0
}

// expectedSize is the total the progress bar assumes: the known remote
// size, or -1 (unknown) when HEAD told us nothing.
func expectedSize(remote int64, haveRemote bool) int64 {
	if haveRemote && remote > 0 {
		return remote
	}
	return -1
}

// drainClose consumes a little of a response body whose content is not
// needed and closes it, so the underlying connection can be reused.
func drainClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4<<10))
	_ = body.Close()
}
