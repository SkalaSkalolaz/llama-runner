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

// NoKeySentinel is the -key (or config "openaiKey") value that states the
// external provider needs NO authentication. Requests then carry no
// Authorization header at all, exactly like an unauthenticated curl call to
// /chat/completions.
//
// It is compared case-insensitively and after trimming spaces, because the
// value is typed by hand into a command line or a JSON file. A provider key
// that literally reads "none" cannot be expressed this way; that is an
// accepted trade-off, and it is why the sentinel is only honoured on values
// the user set for llama-runner (flag, config file, profile) and never on
// OPENAI_API_KEY / LLAMA_RUNNER_OPENAI_KEY, which are shared global state.
const NoKeySentinel = "none"

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
// The second result says the provider was explicitly asked to run WITHOUT a
// key: -key none (see NoKeySentinel, which also works from the config file) or
// -key with an empty value on a flag that was actually given (keyFlagSet).
// That is a decision, not missing data, so neither the environment nor the
// credentials file is consulted on this path — a machine with OPENAI_API_KEY
// exported still talks to the provider unauthenticated when told to.
//
// env and the file system are accessed through injected dependencies
// (env is a lookup function; in production os.Getenv is passed) so tests can
// exercise every priority branch without touching the real home directory.
func ResolveOpenAIKey(keyFlag string, keyFlagSet bool, env func(string) string, home string, stderr io.Writer) (string, bool, error) {
	if wantsKeyless(keyFlag, keyFlagSet) {
		return "", true, nil
	}
	if keyFlag != "" {
		return keyFlag, false, nil
	}
	for _, name := range []string{"LLAMA_RUNNER_OPENAI_KEY", "OPENAI_API_KEY"} {
		if v := strings.TrimSpace(env(name)); v != "" {
			return v, false, nil
		}
	}
	path := CredentialsPath(home)
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnCredCheck, path, err))
		}
		return "", false, nil
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm() != 0o600 {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnCredPerms, path, info.Mode().Perm()))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprint(stderr, i18n.TF(i18n.MsgWarnCredRead, path, err))
		return "", false, nil
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
			return val, false, nil
		}
	}
	return "", false, nil
}

// wantsKeyless reports whether the key value expresses the deliberate choice
// "this provider needs no authentication": the sentinel (NoKeySentinel), or an
// empty value on a -key flag that was actually given.
//
// The two halves differ on purpose. A sentinel is a word, so it means what it
// says wherever the user wrote it for llama-runner — on the command line or in
// the config file / profile, whose "openaiKey" arrives here the same way. An
// EMPTY value carries no meaning by itself: -key "" and "no -key at all" look
// identical by value, which is why keyFlagSet is required to tell them apart.
// Without that distinction an empty value would keep silently meaning "look in
// the environment", and -key "" would be indistinguishable from omitting -key.
//
// Values found in the environment or in the credentials file are not examined
// here: they are stores of keys, not per-run intent, so a value that happens to
// read "none" is passed through as the key it claims to be.
func wantsKeyless(keyFlag string, keyFlagSet bool) bool {
	trimmed := strings.TrimSpace(keyFlag)
	if strings.EqualFold(trimmed, NoKeySentinel) {
		return true
	}
	return keyFlagSet && trimmed == ""
}
