package convert

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
// Каждый payload прогоняется дважды: с дефолтными опциями и с
// ConvertOptions{ImagesToOpenAI: true} (режим `-mmproj`). Второй golden
// (<имя>_mmproj_openai.json) создаётся только для payload'ов с image-блоками —
// для остальных выход совпадает, и дублировать его смысла нет.
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

			got, st, err := ConvertRequest(anthropicReq, ConvertOptions{})
			if err != nil {
				t.Fatalf("ConvertRequest: %v", err)
			}
			checkGolden(t, reqPath, name, "_openai.json", got)

			// Тот же payload с vision-проектором: image-блоки обязаны маппиться.
			if st.DroppedImages > 0 {
				withMmproj, st2, err := ConvertRequest(anthropicReq, ConvertOptions{ImagesToOpenAI: true})
				if err != nil {
					t.Fatalf("ConvertRequest(-mmproj): %v", err)
				}
				if st2.DroppedImages != 0 {
					t.Errorf("-mmproj: image-блоков отброшено %d, want 0", st2.DroppedImages)
				}
				checkGolden(t, reqPath, name, "_mmproj_openai.json", withMmproj)
			}
		})
	}
}

// checkGolden сравнивает результат конвертации с golden-файлом (или пишет его при -update).
func checkGolden(t *testing.T, reqPath, name, suffix string, got map[string]any) {
	t.Helper()

	canonical, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')

	goldenPath := filepath.Join(filepath.Dir(reqPath), name+suffix)
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
}
