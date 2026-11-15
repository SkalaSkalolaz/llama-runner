// Журнал прогона: единственное состояние, позволяющее продолжить
// прерванный запуск. После каждой волны план и результаты завершённых
// шагов целиком перезаписываются в файл (атомарно, через временный файл
// и rename). Файл один — возобновляем всегда последний прогон.
package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// journalVersion guards forward compatibility of the resume file.
const journalVersion = 1

// Journal is the persisted state of one run: the plan itself plus the
// results of every completed step.
type Journal struct {
	Version   int                    `json:"version"`
	Task      string                 `json:"task"`
	WorkDir   string                 `json:"workDir"`
	StartedAt time.Time              `json:"startedAt"`
	Plan      planner.Plan           `json:"plan"`
	Steps     map[string]journalStep `json:"steps"`
}

// journalStep is the persisted half of StepResult (no context, no error
// types — only what resume needs).
type journalStep struct {
	OK       bool          `json:"ok"`
	Alias    string        `json:"alias"`
	Output   string        `json:"output,omitempty"`
	Err      string        `json:"err,omitempty"`
	Attempts int           `json:"attempts"`
	Duration time.Duration `json:"duration"`
}

// NewJournal starts a fresh journal for a run.
func NewJournal(task, workDir string, plan planner.Plan) *Journal {
	return &Journal{
		Version:   journalVersion,
		Task:      task,
		WorkDir:   workDir,
		StartedAt: time.Now(),
		Plan:      plan,
		Steps:     make(map[string]journalStep),
	}
}

// LoadJournal reads the journal at path. A missing file is not an error:
// it returns (nil, nil). A corrupt file returns an error.
func LoadJournal(path string) (*Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// Save writes the journal atomically: temp file + rename, so an
// interrupted run can never leave a half-written journal behind.
func (j *Journal) Save(path string) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Sync records every executed step result so far. Failed steps are
// stored too (for the record), but only successful ones are resumed.
func (j *Journal) Sync(results map[string]StepResult) {
	if j.Steps == nil {
		j.Steps = make(map[string]journalStep, len(results))
	}
	for id, r := range results {
		js := journalStep{
			OK:       r.OK,
			Alias:    r.Alias,
			Output:   r.Output,
			Attempts: r.Attempts,
			Duration: r.Duration,
		}
		if r.Err != nil {
			js.Err = r.Err.Error()
		}
		j.Steps[id] = js
	}
}

// DoneCount is how many successful steps the journal carries.
func (j *Journal) DoneCount() int {
	n := 0
	for _, s := range j.Steps {
		if s.OK {
			n++
		}
	}
	return n
}

// SeedResults converts the journal's successful steps back into
// StepResults keyed by step ID. Failed steps and steps missing from
// the plan are skipped — they simply run again.
func (j *Journal) SeedResults(steps []planner.Step) map[string]StepResult {
	out := make(map[string]StepResult)
	planByID := make(map[string]planner.Step, len(steps))
	for _, s := range steps {
		planByID[s.ID] = s
	}
	for id, js := range j.Steps {
		if !js.OK {
			continue
		}
		step, ok := planByID[id]
		if !ok {
			continue
		}
		out[id] = StepResult{
			Step: step, Alias: js.Alias, OK: true, Output: js.Output,
			Attempts: js.Attempts, Duration: js.Duration,
		}
	}
	return out
}
