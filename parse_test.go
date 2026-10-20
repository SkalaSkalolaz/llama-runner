package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"
)

func TestParseFlags_Defaults(t *testing.T) {
	var buf bytes.Buffer
	cfg, err := parseFlags([]string{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q", cfg.Host)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if !cfg.Jinja {
		t.Error("Jinja should default to true")
	}
	if cfg.ChatTemplate != "auto" {
		t.Errorf("ChatTemplate = %q", cfg.ChatTemplate)
	}
	if cfg.ChatTemplateSet {
		t.Error("ChatTemplateSet should be false")
	}
	if cfg.ProxyAddr != "127.0.0.1:8082" {
		t.Errorf("ProxyAddr = %q, want 127.0.0.1:8082", cfg.ProxyAddr)
	}
}

func TestParseFlags_ExplicitChatTemplate(t *testing.T) {
	var buf bytes.Buffer
	cfg, err := parseFlags([]string{"-chat-template", "/tmp/t.jinja"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ChatTemplateSet {
		t.Error("ChatTemplateSet should be true")
	}
	if cfg.ChatTemplate != "/tmp/t.jinja" {
		t.Errorf("ChatTemplate = %q", cfg.ChatTemplate)
	}
}

func TestParseFlags_Help(t *testing.T) {
	var buf bytes.Buffer
	_, err := parseFlags([]string{"-h"}, &buf)
	if !errors.Is(err, flag.ErrHelp) {
		t.Errorf("err = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(buf.String(), "llama-runner") {
		t.Errorf("usage not printed: %q", buf.String())
	}
}

func TestParseFlags_UnknownFlag(t *testing.T) {
	var buf bytes.Buffer
	_, err := parseFlags([]string{"-nosuchflag"}, &buf)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestValidateConfig_Table(t *testing.T) {
	base := func() *Config {
		return &Config{Host: "127.0.0.1", Port: "8080", McpAddr: "127.0.0.1:8090",
			ReadyTimeout: time.Second, McpProxy: false}
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid", func(c *Config) {}, false},
		{"empty host", func(c *Config) { c.Host = "" }, true},
		{"bad port", func(c *Config) { c.Port = "abc" }, true},
		{"port 0", func(c *Config) { c.Port = "0" }, true},
		{"port 70000", func(c *Config) { c.Port = "70000" }, true},
		{"zero timeout", func(c *Config) { c.ReadyTimeout = 0 }, true},
		{"mcp proxy bad addr", func(c *Config) { c.McpProxy = true; c.McpAddr = "noseparator" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			tt.mutate(c)
			err := validateConfig(c)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveChatTemplate(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		model string
		want  string
	}{
		{"qwen lowercase", "auto", "/m/qwen3.gguf", "~/GGUF/qwen-fixed-chat-template.jinja"},
		{"Qwen mixed case", "auto", "/m/Qwen3.8-Flash.gguf", "~/GGUF/qwen-fixed-chat-template.jinja"},
		{"non-qwen", "auto", "/m/llama3.gguf", ""},
		{"explicit path", "/custom/t.jinja", "/m/qwen.gguf", "/custom/t.jinja"},
		{"explicit empty", "", "/m/qwen.gguf", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveChatTemplate(tt.raw, tt.model); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}