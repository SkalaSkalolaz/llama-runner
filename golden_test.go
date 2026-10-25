package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden перегенерирует ожидаемые файлы:
//
//	go test -run TestGoldenAnthropicToOpenAI -update
var updateGolden = flag.Bool("update", false, "перегенерировать golden-файлы в testdata/anthropic")

// TestGoldenAnthropicToOpenAI — каркас регрессионного барьера конвертера запроса
// (PLAN.md, веха M1). Он намеренно сделан до правок конвертера: сначала фиксируем
// текущее поведение на реальных payload'ах Claude Code, потом меняем логику и
// осознанно обновляем те golden'ы, где поведение должно измениться.
//
// Формат каталога testdata/anthropic/:
//
//	<имя>_request.json — вход (Anthropic Messages API)
//	<имя>_openai.json  — ожидаемый выход (OpenAI chat/completions)
//
// Если golden отсутствует, тест пропускается, а не падает: каркас не должен
// ломать CI до наполнения выборки (см. testdata/anthropic/README.md).
func TestGoldenAnthropicToOpenAI(t *testing.T) {
	requests, err := filepath.Glob(filepath.Join("testdata", "anthropic", "*_request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) == 0 {
		t.Skip("нет payload'ов в testdata/anthropic — см. testdata/anthropic/README.md")
	}

	for _, reqPath := range requests {
		name := strings.TrimSuffix(filepath.Base(reqPath), "_request.json")
		t.Run(name, func(t *testing.T) {
			in, err := os.ReadFile(reqPath)
			if err != nil {
				t.Fatal(err)
			}
			var anthropicReq map[string]any
			if err := json.Unmarshal(in, &anthropicReq); err != nil {
				t.Fatalf("невалидный JSON в %s: %v", reqPath, err)
			}

			got, err := AnthropicToOpenAI(anthropicReq)
			if err != nil {
				t.Fatalf("AnthropicToOpenAI: %v", err)
			}
			canonical, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			canonical = append(canonical, '\n')

			goldenPath := filepath.Join(filepath.Dir(reqPath), name+"_openai.json")
			if *updateGolden {
				if err := os.WriteFile(goldenPath, canonical, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("golden обновлён: %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Skipf("golden %s отсутствует — создайте: go test -run TestGoldenAnthropicToOpenAI -update", goldenPath)
			}
			if string(want) != string(canonical) {
				t.Errorf("конвертация %s изменилась:\n--- want ---\n%s\n--- got ---\n%s", name, want, canonical)
			}
		})
	}
}
