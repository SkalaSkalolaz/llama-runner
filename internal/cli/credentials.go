package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// CredentialsPath returns the file path where llama-runner looks for the
// external provider key when neither the -key flag nor a supporting
// environment variable provides one:
// ~/.config/llama-runner/credentials.
func CredentialsPath(home string) string {
	return filepath.Join(home, ".config", "llama-runner", "credentials")
}

// ResolveOpenAIKey selects the external provider's API key, checking sources
// in strict priority order and returning the first non-empty hit:
//
//	-key flag > LLAMA_RUNNER_OPENAI_KEY > OPENAI_API_KEY >
//	credentials file > "" (no key).
//
// The credentials file (~/.config/llama-runner/credentials) is a simple
// KEY=VALUE list, one pair per line, with "#" comments and optional
// single/double quotes around values; the keys OPENAI_KEY and
// OPENAI_API_KEY are recognised. If the file's permissions are wider than
// 0600 the key is still accepted, but a warning is written to stderr.
//
// A missing, unreadable or malformed file is not fatal — it simply means
// "no key found" — though read failures do produce a stderr warning.
//
// env and the file system are accessed through injected dependencies
// (env is a lookup function; in production os.Getenv is passed) so tests can
// exercise every priority branch without touching the real home directory.
func ResolveOpenAIKey(keyFlag string, env func(string) string, home string, stderr io.Writer) (string, error) {
	if keyFlag != "" {
		return keyFlag, nil
	}
	for _, name := range []string{"LLAMA_RUNNER_OPENAI_KEY", "OPENAI_API_KEY"} {
		if v := strings.TrimSpace(env(name)); v != "" {
			return v, nil
		}
	}
	path := CredentialsPath(home)
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnCredCheck, path, err))
		}
		return "", nil
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm() != 0o600 {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnCredPerms, path, info.Mode().Perm()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnCredRead, path, err))
		return "", nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key != "OPENAI_KEY" && key != "OPENAI_API_KEY" {
			continue
		}
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		if val != "" {
			return val, nil
		}
	}
	return "", nil
}
