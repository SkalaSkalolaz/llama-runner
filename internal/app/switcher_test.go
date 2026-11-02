package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func sp(s string) *string { return &s }

// healthStub — фейковый /health: healthy=true отвечает 200 на /health,
// иначе 404 (сервер «поднялся, но нездоров»). Возвращает порт для cfg.Port.
func healthStub(t *testing.T, healthy bool) string {
	t.Helper()
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy && r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(hs.Close)
	u, err := url.Parse(hs.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port()
}

// switchTestSetup — база для тестов переключения: конфиг с alias "main",
// здоровым health-портом и уже запущенным (sleep-подставкой) сервером.
func switchTestSetup(t *testing.T) (*config.Config, string, *ServerManager, ServerSpec) {
	t.Helper()
	cfg, home := tempLaunchCfg(t)
	cfg.Host = "127.0.0.1"
	cfg.Port = healthStub(t, true)
	cfg.Alias = "main"

	baseSpec, code := SpecForConfig(cfg, home, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("базовый спек не собрался: code=%d", code)
	}
	mgr := NewServerManager(aliveRunner{}, NewShutdown(), io.Discard, io.Discard)
	if err := mgr.Start(baseSpec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(killCurrent(mgr))
	return cfg, home, mgr, baseSpec
}

func TestModelSwitch_HandlesNames(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	cfg.Profiles = []config.Profile{
		{Name: "fast", Alias: "fasty", AliasSet: true},
		{Name: "coder", Model: filepath.Join(home, "coder.gguf")},
	}
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)

	for name, want := range map[string]bool{
		"main":   true, // запущенная модель (local default в меню)
		" fasty": true, // алиас профиля, с пробелом — регистр/пробелы не значат
		"FASTY":  true,
		"coder":  true, // профиль без алиаса именуется по модели
		"ghost":  false,
		"":       false,
	} {
		if got := sw.Handles(name); got != want {
			t.Errorf("Handles(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestModelSwitch_EnsureLoadedCurrentIsNoop(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)

	before := mgr.CurrentCmd()
	alias, err := sw.EnsureLoaded(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if alias != "main" {
		t.Errorf("alias = %q", alias)
	}
	if mgr.CurrentCmd() != before {
		t.Error("повторная загрузка текущей модели не должна перезапускать сервер")
	}
}

func TestModelSwitch_SwitchToProfileAndBack(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	fastModel := filepath.Join(home, "fast.gguf")
	if err := os.WriteFile(fastModel, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Profiles = []config.Profile{{Name: "fast", Alias: "fasty", AliasSet: true, Model: fastModel}}
	cfg.ProfileOverlays = map[string]config.FileConfig{
		"fast": {Model: sp(fastModel), Alias: sp("fasty")},
	}
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)

	oldCmd := mgr.CurrentCmd()
	oldTracked := mgr.Current()

	alias, err := sw.EnsureLoaded(context.Background(), "fasty")
	if err != nil {
		t.Fatalf("переключение: %v", err)
	}
	if alias != "fasty" {
		t.Errorf("alias = %q, want fasty", alias)
	}
	if mgr.CurrentCmd() == oldCmd {
		t.Error("сервер не перезапущен")
	}
	select {
	case <-oldTracked.Done:
	case <-time.After(3 * time.Second):
		t.Error("старый процесс не остановлен переключением")
	}
	if sw.curName != "fasty" {
		t.Errorf("curName = %q", sw.curName)
	}

	// Возврат к локальному дефолту из меню — это запуск исходного спека.
	alias, err = sw.EnsureLoaded(context.Background(), "main")
	if err != nil {
		t.Fatalf("возврат: %v", err)
	}
	if alias != "main" || sw.curName != "main" {
		t.Errorf("после возврата alias=%q curName=%q", alias, sw.curName)
	}
}

func TestModelSwitch_UnknownModelLeavesServerAlone(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)

	before := mgr.CurrentCmd()
	if _, err := sw.EnsureLoaded(context.Background(), "ghost"); err == nil {
		t.Fatal("неизвестная модель должна ошибаться")
	}
	if mgr.CurrentCmd() != before {
		t.Error("сервер не должен был тронуться")
	}
}

func TestModelSwitch_BrokenProfileFailsBeforeStop(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	missing := filepath.Join(home, "absent.gguf")
	cfg.Profiles = []config.Profile{{Name: "bad", Alias: "bady", AliasSet: true, Model: missing}}
	cfg.ProfileOverlays = map[string]config.FileConfig{"bad": {Model: sp(missing), Alias: sp("bady")}}
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)

	before := mgr.CurrentCmd()
	if _, err := sw.EnsureLoaded(context.Background(), "bady"); err == nil {
		t.Fatal("непроверяемый профиль (нет файла модели) должен ошибаться")
	}
	// Ошибка спецификации возникает ДО остановки: работающий сервер цел.
	if mgr.CurrentCmd() != before {
		t.Error("сервер остановлен, хотя новый даже не собрался")
	}
	if sw.curName != "main" {
		t.Errorf("curName изменился при неудаче: %q", sw.curName)
	}
}

func TestModelSwitch_HealthFailureRollsBack(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	sickPort := healthStub(t, false) // всегда 404: сервер «не здоров»
	cfg.Profiles = []config.Profile{{Name: "sick"}}
	cfg.ProfileOverlays = map[string]config.FileConfig{
		"sick": {Port: sp(sickPort), ReadyTimeout: sp("1s")},
	}
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)
	sw.stopTimeout = 2 * time.Second

	if _, err := sw.EnsureLoaded(context.Background(), "sick"); err == nil {
		t.Fatal("нездоровый профиль должен ошибаться")
	}
	if sw.curName != "main" {
		t.Errorf("состояние испорчено неудачным переключением: curName=%q", sw.curName)
	}
	// Откат: текущий процесс — восстановленный прежний, и он здоров.
	if err := mgr.WaitReady(baseSpec); err != nil {
		t.Errorf("после отката сервер не здоров: %v", err)
	}
}

func TestModelSwitch_ModelNamesOrder(t *testing.T) {
	cfg, home, mgr, baseSpec := switchTestSetup(t)
	cfg.Profiles = []config.Profile{
		{Name: "fast", Alias: "fasty", AliasSet: true},
		{Name: "coder", Alias: "codery", AliasSet: true},
	}
	sw := NewModelSwitch(cfg, baseSpec, mgr, home, io.Discard)

	names := sw.ModelNames()
	want := []string{"main", "fasty", "codery"}
	if len(names) != len(want) {
		t.Fatalf("ModelNames = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("ModelNames = %v, want %v", names, want)
		}
	}

	// После переключения имя текущей модели уже в списке — дубликатов нет.
	cfg.ProfileOverlays = map[string]config.FileConfig{"fast": {Alias: sp("fasty")}}
	if _, err := sw.EnsureLoaded(context.Background(), "fasty"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, n := range sw.ModelNames() {
		seen[n]++
	}
	if seen["fasty"] != 1 || seen["main"] != 1 {
		t.Errorf("ModelNames после переключения: %v", sw.ModelNames())
	}
}
