package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/logrotate"
)

// ServerLog is the sink llama-server (and mcp-search-server, and the proxy)
// write their diagnostics to. Writer is what goes into cmd.Stdout/cmd.Stderr;
// Name is the path reported under -v ("" = no file, output stays on the
// console); Close releases the underlying file or rotating writer and must be
// called when the launcher is done, so the sink outlives the child process.
//
// Rotation rules: -quiet-server with -log-max-mb > 0 uses a
// logrotate.RotatingWriter (size-capped, -log-keep old files kept);
// -log-max-mb 0 keeps a plain append-mode file. In -server mode, or without
// -quiet-server, there is no file at all and the child inherits the launcher's
// streams.
type ServerLog struct {
	Writer io.Writer
	File   *os.File
	Name   string
	closer func()
}

// OpenServerLog builds the sink described above and returns the exit code run()
// should use when it fails: 1 for an uncreatable log directory or an
// unopenable file, 2 when the file exists but its mode could not be restricted
// to 0600 (a secret-bearing log left world-readable is treated as a
// configuration-level failure). On success Writer is nil exactly when the
// child must inherit stdout/stderr.
func OpenServerLog(cfg *config.Config, home string, stdout, stderr io.Writer) (ServerLog, int) {
	sl := ServerLog{}
	if cfg.ServerOnly || !cfg.QuietServer {
		return sl, 0
	}
	logPath := config.ExpandHome(cfg.LogFile, home)
	if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o700); mkErr != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrLogDirCreate, filepath.Dir(logPath), mkErr))
		return sl, 1
	}
	if cfg.LogMaxMB > 0 {
		rw, rwErr := logrotate.NewRotatingWriter(logPath, int64(cfg.LogMaxMB)*1024*1024, cfg.LogKeep, 0o600)
		if rwErr != nil {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgErrLogOpenRotating, logPath, rwErr))
			return sl, 1
		}
		sl.Writer = rw
		sl.Name = logPath
		sl.closer = func() { _ = rw.Close() }
		return sl, 0
	}
	f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if openErr != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrLogOpen, logPath, openErr))
		return sl, 1
	}
	if chErr := f.Chmod(0o600); chErr != nil {
		_ = f.Close()
		fmt.Fprint(stderr, i18n.TF(i18n.MsgErrLogChmod, logPath, chErr))
		return sl, 2
	}
	sl.File = f
	sl.Writer = f
	sl.Name = logPath
	sl.closer = func() { _ = f.Close() }
	return sl, 0
}

// Close releases the sink; safe to call for a sink that opened no file.
func (sl ServerLog) Close() {
	if sl.closer != nil {
		sl.closer()
	}
}

// OpenProxyLog returns the file sink for proxy diagnostics when llama-server
// has no log file of its own: ~/.cache/llama-runner/proxy.log, created with
// mode 0600. Unlike OpenServerLog a failure here is not fatal — the proxy
// simply runs without a log, and -v says so. The returned file (nil when no
// sink could be opened) is what the caller must close.
func OpenProxyLog(home string, stdout io.Writer, verbose bool) (io.Writer, *os.File) {
	logPath := config.ExpandHome("~/.cache/llama-runner/proxy.log", home)
	if mkErr := os.MkdirAll(filepath.Dir(logPath), 0o700); mkErr != nil {
		return nil, nil
	}
	pf, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if openErr != nil {
		return nil, nil
	}
	if verbose {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgProxyLogFile, logPath))
	}
	return pf, pf
}
