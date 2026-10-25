#!/usr/bin/env bash
# smoke.sh — дымовая проверка llama-runner перед релизом (PLAN.md, веха M0).
#
# По умолчанию выполняются только безопасные проверки: формат, сборка, коды
# возврата и пути ошибок. Реальные запуски llama-server выключены — они поднимают
# модель и требуют железо.
#
#   ./scripts/smoke.sh              # безопасные проверки
#   ./scripts/smoke.sh --quick      # без go test -race (быстро)
#   ./scripts/smoke.sh --launch     # + запуск llama-server и проверка остановки
#
# Для --launch можно переопределить пути:
#   LLAMA_RUNNER_BIN=~/.local/bin/llama-server MODEL=~/GGUF/model.gguf ./scripts/smoke.sh --launch

set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

BIN="./llama-runner-smoke"
LLAMA_RUNNER_BIN="${LLAMA_RUNNER_BIN:-$HOME/.local/bin/llama-server}"
MODEL="${MODEL:-$HOME/GGUF/Qwen3.8-35B-A3B-Q4_K_M.gguf}"
QUICK=0
LAUNCH=0
FAILED=0

for arg in "$@"; do
	case "$arg" in
	--quick) QUICK=1 ;;
	--launch) LAUNCH=1 ;;
	-h | --help)
		sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "неизвестный аргумент: $arg (см. --help)" >&2
		exit 2
		;;
	esac
done

pass() { printf '  \033[32mOK\033[0m   %s\n' "$1"; }
fail() {
	printf '  \033[31mFAIL\033[0m %s\n' "$1"
	FAILED=$((FAILED + 1))
}
skip() { printf '  \033[33mSKIP\033[0m %s\n' "$1"; }

# expect_code <ожидаемый код> <название> -- <команда...>
expect_code() {
	local want="$1" name="$2"
	shift 3
	local out code
	out="$("$@" 2>&1)"
	code=$?
	if [ "$code" -eq "$want" ]; then
		pass "$name (код $code)"
	else
		fail "$name: код $code, ожидался $want"
		printf '       %s\n' "$(printf '%s' "$out" | head -3 | tr '\n' '|')"
	fi
}

echo "== 1. Инструментальная часть =="

unformatted="$(gofmt -l .)"
if [ -z "$unformatted" ]; then
	pass "gofmt -l пустой"
else
	fail "gofmt: не отформатированы:"
	printf '       %s\n' "$unformatted"
fi

if go vet ./... >/dev/null 2>&1; then
	pass "go vet ./..."
else
	fail "go vet ./..."
	go vet ./... 2>&1 | sed 's/^/       /'
fi

if [ "$QUICK" -eq 1 ]; then
	skip "go test -race (--quick)"
elif go test -race ./... >/dev/null 2>&1; then
	pass "go test -race ./..."
else
	fail "go test -race ./..."
	go test -race ./... 2>&1 | tail -20 | sed 's/^/       /'
fi

# Стриминговые тесты M2 — отдельным прогоном, чтобы их отсутствие/падение было видно.
# go test -run с несовпадающим шаблоном возвращает 0 («no tests to run»),
# поэтому проверяем по -v, что хотя бы один TestConvertStream_* реально запустился.
streamout="$(go test -run 'TestConvertStream_' -count=1 -v . 2>&1)"
streamfail=0
echo "$streamout" | grep -q '^=== RUN   TestConvertStream_' || streamfail=1
echo "$streamout" | grep -q '^--- FAIL' && streamfail=1
if [ "$streamfail" -eq 0 ]; then
	pass "стриминговые тесты M2 (TestConvertStream_*)"
else
	fail "стриминговые тесты M2 (не найдены или упали)"
	echo "$streamout" | tail -20 | sed 's/^/       /'
fi

if go build -ldflags "-X main.version=$(git describe --tags --always --dirty 2>/dev/null)" -o "$BIN" .; then
	pass "сборка с -ldflags -X main.version"
else
	fail "сборка"
	echo
	echo "Итог: $FAILED провалов"
	exit 1
fi

# Ключ и лимиты M3 — отдельным прогоном, чтобы падение было видно.
keyout="$(go test -run 'TestResolveOpenAIKey_|TestNewProxyClient_|TestHandleMessages_TooLarge|TestRun_VerboseDoesNotLeakKey' -count=1 -v . 2>&1)"
keyfail=0
echo "$keyout" | grep -q '^=== RUN   Test' || keyfail=1
echo "$keyout" | grep -q '^--- FAIL' && keyfail=1
if [ "$keyfail" -eq 0 ]; then
	pass "M3: ключ, лимиты тела, таймауты"
else
	fail "M3: ключ/лимиты/таймауты"
	echo "$keyout" | tail -20 | sed 's/^/       /'
fi

# M4: аутентификация прокси — отдельным прогоном.
authout="$(go test -run 'TestProxyAuthMiddleware_|TestProxy_AuthRequired_|TestShouldWarnProxyExposure|TestRequestToken|TestIsLoopbackHost|TestBuildClaudeEnv_(ProxyToken|NoProxyToken)' -count=1 -v . 2>&1)"
authfail=0
echo "$authout" | grep -q '^=== RUN   Test' || authfail=1
echo "$authout" | grep -q '^--- FAIL' && authfail=1
if [ "$authfail" -eq 0 ]; then
    pass "M4: аутентификация прокси"
else
    fail "M4: аутентификация прокси (не найдены или упали)"
    echo "$authout" | tail -20 | sed 's/^/       /'
fi

echo
echo "== 2. Коды возврата и пути ошибок =="

version="$("$BIN" -version 2>&1)"
if [ -n "$version" ]; then
	pass "-version печатает версию: $version"
else
	fail "-version вернул пустую строку"
fi

expect_code 0 "-h выходит с кодом 0" -- "$BIN" -h
expect_code 2 "неизвестный флаг отклоняется" -- "$BIN" "-no-such-flag"
expect_code 2 "-openai без -key отклоняется" -- "$BIN" -openai https://api.example.com/v1
expect_code 2 "некорректный -reasoning отклоняется" -- "$BIN" -reasoning maybe

# -bin указываем на существующий файл, чтобы проверка модели сработала раньше запуска.
fake_bin="$(mktemp)"
expect_code 1 "отсутствующая модель -> ошибка, а не запуск" -- \
	"$BIN" -bin "$fake_bin" -model /nonexistent/model.gguf
rm -f "$fake_bin"

echo
echo "== 3. Справка не противоречит коду =="

if "$BIN" -h 2>&1 | grep -q '^  llama-runner \[флаги\]'; then
	pass "в справке имя бинаря llama-runner"
else
	fail "в справке не найдено 'llama-runner [флаги]'"
fi

echo
if [ "$LAUNCH" -eq 1 ]; then
	echo "== 4. Реальный запуск (--launch) =="
	if [ ! -x "$LLAMA_RUNNER_BIN" ]; then
		skip "llama-server не найден: $LLAMA_RUNNER_BIN (LLAMA_RUNNER_BIN=... )"
	elif [ ! -f "$MODEL" ]; then
		skip "модель не найдена: $MODEL (MODEL=...)"
	else
		port="$((20000 + RANDOM % 20000))"
		"$BIN" -server -bin "$LLAMA_RUNNER_BIN" -model "$MODEL" -port "$port" >"/tmp/llama-runner-smoke-$port.log" 2>&1 &
		launcher_pid=$!
		echo "  launcher pid=$launcher_pid, ждём /health на 127.0.0.1:$port (до 180 с)..."

		ready=0
		for _ in $(seq 1 180); do
			if curl -fsS "http://127.0.0.1:$port/health" >/dev/null 2>&1; then
				ready=1
				break
			fi
			sleep 1
		done
		if [ "$ready" -eq 1 ]; then
			pass "llama-server стал готов по /health"
		else
			fail "llama-server не стал готов: см. /tmp/llama-runner-smoke-$port.log"
		fi

		# Двойной Ctrl-C: второй сигнал не должен ломать остановку и оставлять сирот.
		kill -INT "$launcher_pid" 2>/dev/null
		sleep 1
		kill -INT "$launcher_pid" 2>/dev/null
		wait "$launcher_pid" 2>/dev/null
		if kill -0 "$launcher_pid" 2>/dev/null; then
			fail "launcher жив после двух SIGINT"
			kill -TERM "$launcher_pid" 2>/dev/null
		else
			pass "launcher завершился после двух SIGINT"
		fi
		sleep 2
		orphans="$(pgrep -f 'llama-server.*--port '"$port" || true)"
		if [ -n "$orphans" ]; then
			fail "остались сироты llama-server: $orphans"
		else
			pass "сирот llama-server нет"
		fi
	fi
else
	echo "Пропущено: реальные запуски (--launch)."
fi

echo
echo "Ручная часть (автоматизировать нельзя, прогоняйте перед релизом):"
cat <<'MANUAL'
  [ ] ./llama-runner                       — Claude Code поднялся, tool use работает
  [ ] ./llama-runner -openai ... -key ... -model-openai ...
      — /model llama-server идёт локально, остальное во внешний провайдер
  [ ] ./llama-runner -webui-mcp-proxy      — web_search доступна из Claude Code
  [ ] Ctrl-C во время работы Claude Code   — остановка без висящих процессов
  [ ] Длинная генерация > 5 минут          — стрим не обрывается (актуально после M3)

M1 — фиксы конвертера запроса. Диагностика при -v больше не засоряет TUI:

  Вариант 1 (интерактивно, рекомендован): ./-v без пайпа — TUI Claude Code в
  терминале, [proxy]-строки в лог-файле llama-server (default
  ~/.cache/llama-runner/llama-server.log):

      ./llama-runner -v -openai ... -model-openai ... -key ... -webui-mcp-proxy

      # проверить в логе:
      grep '\[proxy\]' ~/.cache/llama-runner/llama-server.log | tail -20

  Вариант 2 (с пайпом): TUI уходит в /dev/tty, диагностика — в tee-файл.
  ВАЖНО: [proxy]-строки при этом НЕ идут в tee (иначе tee печатает их в тот
  же TTY, что рисует Claude Code, и TUI ломается). Они всегда пишутся в лог
  llama-server:

      ./llama-runner -v ... 2>&1 | tee /tmp/llama-runner-m1.log
      grep '\[proxy\]' ~/.cache/llama-runner/llama-server.log | tail -20

      # в tee-логе в начале должна быть строка:
      #   "Claude Code: stdio перенаправлен в /dev/tty (родительские stdout/stderr — не TTY)"

  [ ] Попросить модель вызвать инструмент («посчитай строки в proxy.go через
      Bash»). В логе на втором запросе — строка
      "[proxy] → апстрим: … tool-ответов=1 …" и за ней
      "[proxy] tool[N] tool_call_id=toolu_… content=\"571 proxy.go\"".
      До M1 tool_result с массивом блоков терялся и было бы tool-ответов=0.
  [ ] Уронить инструмент («выполни команду, которой нет»): в логе строка
      "content=\"Error: bash: … not found\"" — префикс Error: виден в логе,
      даже если сама модель пересказала ошибку пользователю без него.
  [ ] Первое сообщение запроса role=system содержит системный промпт Claude Code,
      а не пустой список. До M1 system массивом блоков терялся.
  [ ] Строки "[proxy] отброшено блоков: ..." в обычном текстовом диалоге нет;
      thinking появляется только когда модель реально размышляет; image не
      появляется при запуске с -mmproj.

M2 — стриминг OpenAI → Anthropic. Usage и обрыв апстрима:

  Все проверки — в лог-файле llama-server (тот же, что в M1):
      LOG=~/.cache/llama-runner/llama-server.log

  [ ] Ненулевой usage. Запустить ./llama-runner -v ... и попросить модель
      что-то длинное («напиши 20 строк про X»). После завершения ответа:

          grep '← usage' "$LOG" | tail -3

      Ожидаемо: "[proxy] ← usage: input_tokens=<N>, output_tokens=<M>"
      с N>0 и M>0. До M2 usage всегда был 0, и контекст-учёт в Claude Code
      (индикатор заполнения контекста) врал.

  [ ] Обрыв посреди стрима. Запустить длинную генерацию, затем
      во втором терминале:

          pkill -9 -f 'llama-server.*--port'

      Ожидаемо: Claude Code показывает ошибку («API Error» /
      «Stream interrupted» / аналог), НЕ притворяется, что генерация
      завершилась успешно. До M2 обрыв приводил к фиктивному message_stop,
      и модель «как будто» закончила отвечать на середине фразы.

  [ ] Нормальный стоп не задет. Обычный короткий диалог («скажи привет»)
      завершается штатно, без event: error в ответе. В логе — та же
      строка "← usage: ...", но message_delta с stop_reason=end_turn.

M3 — ключ из env, лимит тела, длинный стрим:

  [ ] Ключ из env (без -key). В отдельном терминале:
        export LLAMA_RUNNER_OPENAI_KEY=sk-...
        ./llama-runner -v -openai https://api.example.com/v1 \
          -model-openai deepseek/deepseek-v4-flash
      Должно стартовать; в выводе -v ключа нет. То же с
        OPENAI_API_KEY=sk-...
      и с файлом ~/.config/llama-runner/credentials (0600):
        OPENAI_KEY=sk-...
      При правах 0644 — предупреждение в stderr, но запуск идёт.

  [ ] Слишком большое тело. Отправить в прокси запрос с телом > 100 МБ
      (например, curl с гигантским JSON или base64-картинкой):
        curl -sS -o /dev/null -w '%{http_code}\n' \
          -X POST http://127.0.0.1:8082/v1/messages \
          -H 'Content-Type: application/json' \
          --data-binary @/path/to/huge.json
      Ожидаемо: 413 и JSON {"type":"error","error":{"type":"request_too_large",...}}.

  [ ] Длинный стрим > 5 минут. Запустить ./llama-runner -v ... и попросить
      модель очень длинный ответ («напиши 200 строк про X»). Стрим не
      обрывается на 5:00. До M3 Client.Timeout=5m резал его посреди ответа.

  [ ] Зависший апстрим отсекается. Если возможно — подсунуть upstream,
      который принимает TCP и молчит. Ответ должен прийти через ~30 с
      (ResponseHeaderTimeout), не через 5 минут.

M4 — аутентификация прокси:

  [ ] Обычный запуск без -proxy-token: Claude Code ходит на прокси как
      раньше (ANTHROPIC_AUTH_TOKEN=local-llama, прокси токен не проверяет).

  [ ] С -proxy-token:
        ./llama-runner -v -proxy-token mysecret -openai ... -key ... -model-openai ...
      В -v-выводе строка "ANTHROPIC_AUTH_TOKEN=<configured via -proxy-token, hidden>".
      Claude Code работает (запросы проходят).

  [ ] Запрос без токена получает 401 в Anthropic-формате:
        curl -sS -o /dev/null -w '%{http_code}\n' \
          -X POST http://127.0.0.1:8082/v1/messages \
          -H 'Content-Type: application/json' \
          -d '{"model":"llama-server","messages":[{"role":"user","content":"hi"}]}'
      Ожидаемо: 401, JSON {"type":"error","error":{"type":"authentication_error",...}}.

  [ ] С тем же токеном — 200:
        curl -sS -o /dev/null -w '%{http_code}\n' \
          -X POST http://127.0.0.1:8082/v1/messages \
          -H 'Authorization: Bearer mysecret' \
          -H 'Content-Type: application/json' \
          -d '{"model":"llama-server","messages":[{"role":"user","content":"hi"}]}'
      И то же с x-api-key: mysecret — тоже 200.

  [ ] Не-loopback без токена и без -proxy-allow-remote:
        ./llama-runner -proxy-addr 0.0.0.0:8082 -openai ... -key ... -model-openai ...
      Предупреждение в stderr, запуск идёт (не hard-fail).

  [ ] -proxy-allow-remote снимает предупреждение.
MANUAL

echo
if [ "$FAILED" -eq 0 ]; then
	echo "Итог: все автоматические проверки пройдены"
	exit 0
fi
echo "Итог: $FAILED провалов"
exit 1