package download

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// This module owns one responsibility: splitting the composite -download
// value "<local-dir><url>" into its two parts. The value is passed as a
// single argument (so a URL containing spaces or query parameters needs no
// quoting tricks), and the split point is the first occurrence of an
// http(s):// scheme — directory prefixes never contain a scheme, so the
// first hit is always where the URL begins.

// URL schemes recognised inside the composite value. Matched
// case-insensitively; "http://" is not a substring of "https://" (the colon
// must follow "http" directly), so both markers can simply be searched and
// the earlier hit wins.
const (
	schemeHTTP  = "http://"
	schemeHTTPS = "https://"
)

// ParseSpec splits a -download value of the form <local-dir><url> into the
// destination directory and the model URL, e.g.
//
//	~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf
//	→ dir "/home/u/GGUF/", url
//	  "https://huggingface.co/org/repo/resolve/main/model.gguf".
//
// The value is split first and the leading "~" is expanded on the
// directory part only (expanding the composite value would let
// filepath.Join collapse the "//" of the URL scheme), and the directory is
// made absolute. The returned
// directory always ends with a path separator so callers may append file
// names directly. The URL is returned verbatim — Hugging Face treats the
// owner/repo part as case-sensitive and query parameters must survive
// untouched.
//
// Errors (localized): the value contains no http(s) scheme
// (MsgErrDownloadNoURL), the directory prefix is empty
// (MsgErrDownloadNoDir), or the URL part is not an absolute http(s) URL
// (MsgErrDownloadSpec).
func ParseSpec(value, home string) (dir, urlStr string, err error) {
	idx := schemeIndex(value)
	if idx < 0 {
		return "", "", i18n.Errorv(i18n.MsgErrDownloadNoURL)
	}
	// Split before expanding, and expand only the directory part:
	// ExpandHome → filepath.Join would collapse the "//" of the URL
	// scheme if it ever saw the composite value.
	dirPart, urlPart := config.ExpandHome(value[:idx], home), value[idx:]
	if dirPart == "" {
		return "", "", i18n.Errorv(i18n.MsgErrDownloadNoDir)
	}
	u, perr := url.Parse(urlPart)
	if perr != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", i18n.Errorv(i18n.MsgErrDownloadSpec, urlPart)
	}
	abs, aerr := filepath.Abs(dirPart)
	if aerr != nil {
		return "", "", i18n.Errorf(i18n.MsgErrDownloadSpec, aerr, dirPart)
	}
	// Abs cleans the path and drops any trailing separator, so the
	// "dir ends with a separator" guarantee is restored after it, not
	// before: a value without the trailing "/" gets one, "/" stays "/".
	if !strings.HasSuffix(abs, string(filepath.Separator)) {
		abs += string(filepath.Separator)
	}
	return abs, urlPart, nil
}

// JoinDir appends a file name to a directory path, tolerating both the
// separator-terminated form ParseSpec returns and a plain directory path.
func JoinDir(dir, name string) string {
	if dir == "" {
		return name
	}
	if dir[len(dir)-1] == filepath.Separator {
		return dir + name
	}
	return filepath.Join(dir, name)
}

// schemeIndex returns the byte index of the first http:// or https://
// occurrence (case-insensitive) in s, or -1 when neither is present.
func schemeIndex(s string) int {
	lower := strings.ToLower(s)
	i, is := strings.Index(lower, schemeHTTP), strings.Index(lower, schemeHTTPS)
	switch {
	case i < 0:
		return is
	case is < 0:
		return i
	case i < is:
		return i
	default:
		return is
	}
}
