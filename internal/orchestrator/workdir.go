package orchestrator

import "github.com/SkalaSkalolaz/llama-runner/internal/config"

// WorkDir exposes the agent working-directory resolution to callers outside
// the package: the planner must see the same directory the steps run in.
func WorkDir(cfg *config.Config, home string) (string, error) {
	return stepWorkDir(Options{Cfg: cfg, Home: home})
}
