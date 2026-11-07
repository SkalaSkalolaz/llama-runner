# bash completion for llama-runner
_llama_runner() {
    local cur prev words cword
    _init_completion -n = 2>/dev/null || {
        COMPREPLY=()
        cur="${COMP_WORDS[COMP_CWORD]}"
    }

    local flags="
        --alias --bin --model --download --host --port
        -c --c -b --b -ub --ub -fa --fa --parallel --tools --jinja
        --ngl --mmproj --temp --top-p --cache-type-k --cache-type-v
        --fit --lv --lm-lazy --reasoning --reasoning-budget
        --spec-type --spec-draft-model --spec-draft-n-max --spec-draft-ngl
        --dir --server --claude-bin --claude-dir --ready-timeout
        --chat-template --log-file --quiet-server --log-max-mb --log-keep
        --webui-mcp-proxy --mcp-addr --mcp-config --mcp-strict
        --openai --key --model-openai --openai-path
        --proxy-addr --proxy-token --proxy-allow-remote
        --config -p --doctor --update --llama-src --install-prefix
        --cuda --cuda-arch --version -v -h --help
    "

    case "$prev" in
        --model|--bin|--log-file|--mcp-config|--config|--claude-bin|--mmproj|--spec-draft-model|--llama-src|--install-prefix|--chat-template)
            _filedir 2>/dev/null || true
            return 0
            ;;
        --p)
            return 0
            ;;
    esac

    COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
}
complete -F _llama_runner llama-runner