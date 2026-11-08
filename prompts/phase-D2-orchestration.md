Phase D.2 — Swap orchestration end-to-end

Complete the auto-mode flow: real swaps when the client changes model.
Add an integration test that exercises the full pipeline (proxy swap,
new server start, request served by new server).
Files you may edit

    internal/app/automode.go (+ up to 80 lines)

    internal/app/swapper.go (+ up to 60 lines — wire Inc/Dec if not done)

    internal/app/automode_test.go (extend, + up to 150 lines)

    internal/app/swapper_test.go (extend, + up to 100 lines)

Tasks

D.2.1 — Wire in-flight tracking

Ensure profileSwapper.IncInflight / DecInflight are called from the proxy
handler (already done in Phase B.3, verify).

D.2.2 — End-to-end test

TestRunAuto_SwapBetweenProfiles:

    Two profiles p1, p2 in a temp config, each with a distinct alias.

    Use recordingRunner that returns sleep 2 for both.

    Start runAuto in a goroutine with -planner p1.

    Wait until server is up.

    Send /v1/messages with model=p1 — should be served without swap.

    Send /v1/messages with model=p2 — triggers swap; second request
    returns after the new server is up.

    Assert: both requests got responses, recordingRunner recorded 2 server
    starts, swap happened once.

D.2.3 — Failure path test

TestRunAuto_SwapFails_Returns502 — recordingRunner fails on the second
start. /v1/messages with model=p2 returns 502.
Acceptance

    go test -race ./internal/app/... ok, e2e test passes.

    llama-runner -auto -v runs manually and swaps when /model is changed
    (manual check, not automated).