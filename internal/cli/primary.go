package cli

import "github.com/SkalaSkalolaz/llama-runner/internal/config"

// PrimaryModel returns the model that fills the four standard /model slots
// (Opus/Sonnet/Haiku/Fable) and the --model pin at startup:
//
//   - external provider configured → cfg.OpenAIModel (the external model
//     the user actually wants as default in hybrid mode);
//   - otherwise → cfg.Alias, already resolved from -p / -model / root.
//
// Values in standard slots are sent bare; the proxy uses them to route to
// whichever server is currently running, never to trigger a swap.
func PrimaryModel(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.OpenAIProviderConfigured() {
		return cfg.OpenAIModel
	}
	return cfg.Alias
}
