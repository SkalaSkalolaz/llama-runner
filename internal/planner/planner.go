package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

const (
	// defaultMaxTokens bounds the plan itself. A 32-step plan with short
	// goals fits comfortably; the cap stops a runaway generation.
	defaultMaxTokens = 4096
	// headerTimeout mirrors the proxy's local client: llama-server sends
	// no response headers until prefill is done, which on a large model
	// with a hybrid CPU/GPU offload can take minutes.
	headerTimeout = 5 * time.Minute
	// maxRepairs is how many times an invalid answer is sent back for
	// correction before the fallback plan is used.
	maxRepairs = 1
)

// Options configures a Planner.
type Options struct {
	// BaseURL is the llama-server base URL (no trailing slash).
	BaseURL string
	// Alias is the planner's own alias, sent as the request's model name.
	Alias string
	// Models is the worker catalog offered to the planner.
	Models []Model
	// Client is optional; nil builds one with headerTimeout.
	Client *http.Client
	// MaxTokens is optional; 0 means defaultMaxTokens.
	MaxTokens int
}

// Planner asks the local "auto" model for a plan and validates the answer.
type Planner struct {
	opts   Options
	client *http.Client
}

// New builds a Planner from opts.
func New(opts Options) *Planner {
	c := opts.Client
	if c == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.ResponseHeaderTimeout = headerTimeout
		c = &http.Client{Transport: t}
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = defaultMaxTokens
	}
	return &Planner{opts: opts, client: c}
}

// Result reports what Build produced and how.
type Result struct {
	Plan Plan
	// Attempts is the number of model calls made (1 = first answer valid).
	Attempts int
	// Repaired is true when the plan became valid only after a repair round.
	Repaired bool
	// Fallback is true when no valid plan was obtained and a single-step
	// plan on the planner itself was substituted. This is a WARNING, not a
	// failure: the caller must say so out loud.
	Fallback bool
	// LastErr is the validation error of the final attempt (set when
	// Fallback is true).
	LastErr error
}

// Build asks the planner for a plan for task. Transport and HTTP failures
// are returned as errors; an answer that cannot be parsed or validated is
// sent back for one repair round, and only then replaced by the fallback.
func (p *Planner) Build(ctx context.Context, task string) (Result, error) {
	aliases := make([]string, 0, len(p.opts.Models))
	for _, m := range p.opts.Models {
		aliases = append(aliases, m.Alias)
	}
	msgs := []message{
		{Role: "system", Content: systemPrompt(p.opts.Models, p.opts.Alias)},
		{Role: "user", Content: strings.TrimSpace(task)},
	}
	var lastErr error
	for attempt := 1; attempt <= maxRepairs+1; attempt++ {
		reply, err := p.complete(ctx, msgs)
		if err != nil {
			return Result{}, err
		}
		plan, verr := Parse(reply)
		if verr == nil {
			verr = plan.Validate(aliases)
		}
		if verr == nil {
			return Result{Plan: plan, Attempts: attempt, Repaired: attempt > 1}, nil
		}
		lastErr = verr
		msgs = append(msgs,
			message{Role: "assistant", Content: reply},
			message{Role: "user", Content: repairPrompt(verr)})
	}
	return Result{
		Plan:     fallbackPlan(task, p.opts.Alias),
		Attempts: maxRepairs + 1,
		Fallback: true,
		LastErr:  lastErr,
	}, nil
}

// fallbackPlan is the honest minimum: one step, on the planner itself, with
// the task verbatim as the goal. It never invents work the model did not ask
// for, and Result.Fallback tells the caller to warn the user.
func fallbackPlan(task, alias string) Plan {
	return Plan{Steps: []Step{{
		ID: "1", Role: "general", Goal: strings.TrimSpace(task), Alias: alias,
	}}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	// Temperature is pinned to 0: planning must be reproducible, and the
	// profile's sampling temperature is meant for creative work, not for
	// schema adherence.
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	Stream      bool    `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// complete performs one non-streaming chat/completions call against the
// local server. Only message.content is read: a reasoning model's
// reasoning_content is not part of the plan.
func (p *Planner) complete(ctx context.Context, msgs []message) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: p.opts.Alias, Messages: msgs, MaxTokens: p.opts.MaxTokens,
	})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(p.opts.BaseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", i18n.Errorv(i18n.MsgErrPlannerHTTP, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", i18n.Errorf(i18n.MsgErrPlanJSON, err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", i18n.Errorv(i18n.MsgErrPlannerEmpty)
	}
	return out.Choices[0].Message.Content, nil
}
