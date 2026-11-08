Phase B.1 — ServerManager.Stop and SwapTo

Add the ability to stop and replace the running llama-server. No proxy
integration yet — just the manager methods and their tests.
Files you may edit

    internal/app/servermgr.go (+ up to 120 lines)

    internal/app/servermgr_test.go (extend, + up to 120 lines)

    internal/i18n/messages.go (+ up to 5 keys)

Tasks

B.1.1 — Stop() error

Add to ServerManager:

// Stop terminates the current llama-server, waiting up to gracePeriod for
// SIGTERM to take effect, then escalating to SIGKILL.
func (m *ServerManager) Stop() error

Semantics:

    If no current process — return nil.

    Signal the process group SIGTERM (use syscall.Kill(-pid, SIGTERM) — same
    pattern as shutdown.signal).

    Wait up to 5 seconds for tracked.Done.

    If still alive — SIGKILL group.

    Set current and tracked to nil.

    Idempotent: second call after successful first -> nil, no panic.

B.1.2 — SwapTo(spec ServerSpec) (ServerSpec, error)

// SwapTo replaces the current server with one built from spec. It stops
// the old one, starts the new one and waits for it to become ready.
// Returns the spec that is now running (with a possibly different Port
// from the original — see below).
func (m *ServerManager) SwapTo(spec ServerSpec) (ServerSpec, error)

Steps:

    Allocate a free port via net.Listen("tcp", "127.0.0.1:0") -> get port
    -> Close(). Rewrite spec.Args to replace --port <old> with --port <new>.
    Rewrite spec.HealthURL host:port accordingly. This avoids TIME_WAIT races
    on the old port.

    m.Stop().

    m.Start(newSpec).

    m.WaitReady(newSpec).

    Return newSpec, nil on success.

    On any error: return zero spec and wrap the error with a meaningful
    message; the old server is already stopped (document this).

B.1.3 — Tests

    TestServerManager_Stop_NoProcess — nil.

    TestServerManager_Stop_RunningProcess — starts aliveRunner, Stop succeeds within 5s.

    TestServerManager_Stop_Idempotent — twice in a row.

    TestServerManager_SwapTo_Success — start A, swap to B, B is current, A is dead.

    TestServerManager_SwapTo_FreePortDiffers — swap produces different port.

    TestServerManager_SwapTo_StartFails — new binary invalid -> error, old is gone.

Use recordingRunner pattern already present in server_alias_test.go for
observability; extend aliveRunner in servermgr_test.go if needed.
Acceptance

    All existing servermgr_test.go tests still pass unchanged.

    New tests pass with -race.

    go vet ./internal/app/... clean.