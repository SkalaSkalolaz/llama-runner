// Command llama-runner launches a local llama-server with a GGUF model and,
// by default, hands control over to Claude Code configured to talk to that
// server through ANTHROPIC_* environment variables. It can additionally run
// an mcp-search-server (web search), a local proxy that routes between the
// local llama-server and an external OpenAI-compatible provider, and offers
// llama.cpp update (-update) and diagnostics (-doctor) modes.
//
// All behaviour lives in library packages under internal/; this file is the
// process entry point only: it injects the real command runner and the build
// version into app.Run and turns the returned status into the exit code. The
// -h/--help text is installed by the flag parser (see cli's usage.go), so the
// executable never has to know about it.
package main

import (
	"os"

	"github.com/SkalaSkalolaz/llama-runner/internal/app"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// version is the human-readable program version reported by -version and
// -doctor. Release builds override it at link time with
// -ldflags "-X main.version=<git describe>"; this literal is the fallback
// used for local development builds. It is passed down to the run loop and
// the doctor report as data, so no library package has to depend on the
// executable.
var version = "1.6"

// main is the process entry point. It delegates everything to app.Run (which
// is fully testable with injected streams and runner) and turns the returned
// status code into the process exit code.
func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr, runner.RealRunner{}, version))
}
