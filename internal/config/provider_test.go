package config

import "testing"

func TestOpenAIProviderConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"nothing", Config{}, false},
		{"url+model+key", Config{OpenAIBaseURL: "https://x/v1", OpenAIModel: "m", OpenAIKey: "k"}, true},
		{"url+model, no key at all", Config{OpenAIBaseURL: "https://x/v1", OpenAIModel: "m"}, false},
		{
			"url+model, keyless on purpose",
			Config{OpenAIBaseURL: "https://x/v1", OpenAIModel: "m", OpenAIKeyless: true},
			true,
		},
		{"url+key, no model", Config{OpenAIBaseURL: "https://x/v1", OpenAIKey: "k"}, false},
		{"model+key, no url", Config{OpenAIModel: "m", OpenAIKey: "k"}, false},
		{"keyless without a url is not a provider", Config{OpenAIModel: "m", OpenAIKeyless: true}, false},
		{"url alone", Config{OpenAIBaseURL: "https://x/v1"}, false},
		{
			"key wins over a stray keyless flag",
			Config{OpenAIBaseURL: "https://x/v1", OpenAIModel: "m", OpenAIKey: "k", OpenAIKeyless: true},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.OpenAIProviderConfigured(); got != tt.want {
				t.Errorf("OpenAIProviderConfigured() = %v, want %v (%+v)", got, tt.want, tt.cfg)
			}
		})
	}
}
