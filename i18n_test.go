package main

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// --- language selection ---

func TestT_LocaleSelection(t *testing.T) {
	// msgDone — простое сообщение без спецификаторов: "Готово." / "Done."
	wantRU := messages[msgDone].ru
	wantEN := messages[msgDone].en

	// NB: isRussianLocale проверяет ОБЕ переменные: LC_ALL=ru даёт ru,
	// иначе LANG=ru тоже даёт ru (LC_ALL=C его не «перебивает»).
	tests := []struct {
		name     string
		lcAll    string
		lang     string
		language string
		want     string
	}{
		{"LC_ALL russian", "ru_RU.UTF-8", "C", "", wantRU},
		{"LC_ALL russian beats LANG english", "ru_RU.UTF-8", "en_US.UTF-8", "", wantRU},
		{"LANG russian, LC_ALL empty", "", "ru_RU.UTF-8", "", wantRU},
		{"LANG russian with LC_ALL=C", "C", "ru_RU.UTF-8", "", wantRU},
		{"LANGUAGE ignored", "", "", "ru", wantEN},
		{"all unset-ish → english", "C", "C", "", wantEN},
		{"ukrainian LANG → english", "", "uk_UA.UTF-8", "", wantEN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LC_ALL", tt.lcAll)
			t.Setenv("LANG", tt.lang)
			t.Setenv("LANGUAGE", tt.language)
			if got := T(msgDone); got != tt.want {
				t.Errorf("T(msgDone) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestT_UnknownKeyReturnsKey(t *testing.T) {
	for _, lc := range []string{"ru_RU.UTF-8", "C"} {
		t.Setenv("LC_ALL", lc)
		const key = "no_such_message_key_xyz"
		if got := T(key); got != key {
			t.Errorf("T(%q) = %q, want the key itself", key, got)
		}
	}
}

// --- helpers ---

func TestTF_AppendsNewline(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	t.Setenv("LANG", "C")
	if got := TF(msgStatus, 503); got != "статус 503\n" {
		t.Errorf("TF(msgStatus, 503) ru = %q, want %q", got, "статус 503\n")
	}
	t.Setenv("LC_ALL", "C")
	if got := TF(msgStatus, 503); got != "status 503\n" {
		t.Errorf("TF(msgStatus, 503) en = %q", got)
	}
}

func TestErrorf_PreservesCauseChain(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	sentinel := errors.New("boom")
	err := errorf(msgErrConfigRead, sentinel, "/tmp/cfg.json")
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is must find the wrapped cause: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "/tmp/cfg.json") || !strings.HasSuffix(msg, ": boom") {
		t.Errorf("unexpected message: %q", msg)
	}
	if strings.Contains(msg, "%!") {
		t.Errorf("format leftovers in %q", msg)
	}
}

func TestErrorv_NoWrappingNoNewline(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	err := errorv(msgErrPortBad, "abc")
	if strings.HasSuffix(err.Error(), "\n") {
		t.Errorf("error must not end with a newline: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "abc") {
		t.Errorf("args must be consumed: %q", err.Error())
	}
}

// --- catalog consistency ---

// formatSpecRe matches fmt format directives (%% or %...verb).
var formatSpecRe = regexp.MustCompile(`\%\%|%(?:\d+\$)?[-+ #0]*\d*(?:\.\d+)?[a-zA-Z]`)

func TestCatalog_FormatSpecifiersMatch(t *testing.T) {
	if len(messages) == 0 {
		t.Fatal("catalog is empty")
	}
	for key, m := range messages {
		ru := formatSpecRe.FindAllString(m.ru, -1)
		en := formatSpecRe.FindAllString(m.en, -1)
		if !slices.Equal(ru, en) {
			t.Errorf("key %q: ru specs %v != en specs %v (ru=%q en=%q)", key, ru, en, m.ru, m.en)
		}
	}
}

func TestCatalog_NoTrailingNewline(t *testing.T) {
	// TF добавляет перевод строки сам; шаблон с "\n" на конце дал бы двойную.
	// Ведущие "\n" (сообщения о сигналах) разрешены.
	for key, m := range messages {
		if strings.HasSuffix(m.ru, "\n") || strings.HasSuffix(m.en, "\n") {
			t.Errorf("key %q: template must not end with a newline (TF appends one)", key)
		}
	}
}

func TestCatalog_EntriesComplete(t *testing.T) {
	for key, m := range messages {
		if m.ru == "" || m.en == "" {
			t.Errorf("key %q: empty translation ru=%q en=%q", key, m.ru, m.en)
		}
	}
}
