// Package i18n implements the launcher's lightweight localization: every message
// printed to the terminal (progress lines, warnings, errors, the -doctor
// report, proxy diagnostics) or surfaced as an error is looked up here and
// rendered in the console language — Russian when IsRussianLocale reports a
// Russian locale, English otherwise.
//
// Usage at call sites:
//
//	fmt.Fprint(stderr, TF(MsgKey, arg1, arg2))         // localized line to a stream
//	err := Errorf(MsgKey, err)                          // wrapped %w error
//	err := Errorv(MsgKey, args...)                        // %v-style error
//
// TF always appends a trailing newline; templates must not carry "\n"
// themselves. Templates keep the original format specifiers (%s, %q, %d, %v,
// %w), and the same argument list is passed to both language variants, so
// every entry's ru and en templates must use identical specifiers.
//
// Tests must not depend on the ambient locale: pin it explicitly with
// t.Setenv("LC_ALL", "ru_RU.UTF-8") (or "C") when asserting message text.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// IsRussianLocale reports whether the console locale is Russian.
//
// LC_ALL wins over LANG, matching POSIX precedence: a locale set in LC_ALL
// decides the answer, even when LANG disagrees. "C", "POSIX" and "C.UTF-8"
// mean "no language preference" and are skipped so the next variable is
// consulted. LANGUAGE is deliberately ignored — it selects gettext message
// translations, not the console locale.
func IsRussianLocale() bool {
	for _, v := range []string{os.Getenv("LC_ALL"), os.Getenv("LANG")} {
		if v == "" || v == "C" || v == "POSIX" || v == "C.UTF-8" {
			continue
		}
		return strings.HasPrefix(v, "ru")
	}
	return false
}

// i18nMessage is one message in all supported languages. The ru and en
// templates must contain the same set of format specifiers.
type i18nMessage struct{ ru, en string }

// T returns the localized message template for key. An unknown key returns
// the key itself, which makes missing translations visible instead of
// crashing or printing nothing.
func T(key string) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	if IsRussianLocale() {
		return m.ru
	}
	return m.en
}

// TF renders the localized template for key with args (fmt.Sprintf) and
// appends a newline. Use it with fmt.Fprint to emit a localized line:
//
//	fmt.Fprint(stderr, TF(MsgKey, value))
func TF(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...) + "\n"
}

// Errorf wraps cause into a localized error of the form "<message>: <cause>".
// The cause is always attached with %w, preserving errors.Is/As chains, so
// templates never carry a format specifier for it.
func Errorf(key string, cause error, args ...any) error {
	return fmt.Errorf("%s: %w", strings.TrimSuffix(TF(key, args...), "\n"), cause)
}

// Errorv builds a localized error from the template itself; the template's
// format specifiers consume args. Use it when there is no single wrapped
// cause (validation failures and similar).
func Errorv(key string, args ...any) error {
	return fmt.Errorf("%s", strings.TrimSuffix(TF(key, args...), "\n"))
}
