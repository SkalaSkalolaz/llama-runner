# Golden corpus for the Anthropic -> OpenAI request converter

`go test -run TestGoldenAnthropicToOpenAI` (see `internal/convert/golden_test.go`)
reads this directory. Tests run with the package directory as the working
directory, so the path is package-relative: `testdata/anthropic`.

## Naming

    <NAME>_request.json       input: an Anthropic Messages API request body
    <NAME>_openai.json        expected output: an OpenAI chat/completions request body
    <NAME>_mmproj_openai.json expected output in -mmproj mode (only for payloads that contain image blocks)

## How to fill the corpus

1. Capture real Claude Code requests rather than writing them by hand: log
   the proxy's incoming body with `-v`, or copy the body out of
   `llama-server`'s log, then pretty-print the JSON.
2. Add only `<NAME>_request.json`; the expected files are generated:

   ```bash
   go test -run TestGoldenAnthropicToOpenAI -update
   ```

3. Review the generated output by hand before committing (any converter bug
   present at that moment becomes the baseline), then commit the request and
   its expected output together.
4. When the converter changes on purpose, regenerate only the affected goldens
   and show the diff in the commit message.

## Why a missing golden is a skip, not a failure

The regression barrier was committed before the sample payloads were captured.
A payload without an expected file is skipped, and when the directory holds no
payloads at all the whole test is skipped - so CI stays green until this
corpus is filled in.
