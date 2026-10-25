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

if go build -ldflags "-X main.version=$(git describe --tags --always --dirty 2>/dev/null)" -o "$BIN" .; then
	pass "сборка с -ldflags -X main.version"
else
	fail "сборка"
	echo
	echo "Итог: $FAILED провалов"
	exit 1
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

M1 — фиксы конвертера запроса (прогонять с -v, лог заводить в файл):

    ./llama-runner -v 2>&1 | tee /tmp/llama-runner-m1.log

  [ ] Попросить модель вызвать инструмент (например «посчитай строки в proxy.go
      через Bash»). В логе на втором запросе должна появиться строка
      "[proxy] → апстрим: … tool-ответов=1 …" — до M1 tool_result с массивом
      блоков терялся и было бы tool-ответов=0.
  [ ] Уронить инструмент (например «выполни команды, которой нет»): в content
      сообщения role=tool есть префикс "Error: ".
  [ ] Первое сообщение запроса role=system содержит системный промпт Claude Code,
      а не пустой список. До M1 system массивом блоков терялся.
  [ ] Строки "[proxy] отброшено блоков: ..." в обычном текстовом диалоге нет;
      thinking появляется только когда модель реально размышляет; image не
      появляется при запуске с -mmproj.
MANUAL

echo
if [ "$FAILED" -eq 0 ]; then
	echo "Итог: все автоматические проверки пройдены"
	exit 0
fi
echo "Итог: $FAILED провалов"
exit 1
