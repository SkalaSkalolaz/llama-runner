#compdef llama-runner

_llama_runner() {
    _arguments -s \
        '--alias[model alias]:name:' \
        '--bin[path to llama-server]:file:_files' \
        '--model[path to GGUF model]:file:_files' \
        '--download[download model: <dir><url>]:value:' \
        '--host[server host]:host:' \
        '--port[server port]:port:' \
        '-c[context size]:n:' \
        '-b[batch size]:n:' \
        '-ub[micro-batch size]:n:' \
        '-fa[flash attention]:value:' \
        '--parallel[parallel slots]:n:' \
        '--tools[tools mode]:value:' \
        '--jinja[enable Jinja templates]' \
        '--ngl[layers on GPU]:n:' \
        '--mmproj[multimodal projector]:file:_files' \
        '--temp[temperature]:value:' \
        '--top-p[top-p]:value:' \
        '--cache-type-k[K cache type]:value:' \
        '--cache-type-v[V cache type]:value:' \
        '--fit[--fit mode]:value:(on off)' \
        '--lv[llama.cpp log verbosity]:n:' \
        '--lm-lazy[lazy mmap]' \
        '--reasoning[reasoning mode]:value:(on off auto)' \
        '--reasoning-budget[reasoning token budget]:n:' \
        '--spec-type[spec-decoding type]:value:' \
        '--spec-draft-model[draft model]:file:_files' \
        '--spec-draft-n-max[draft n-max]:n:' \
        '--spec-draft-ngl[draft ngl]:n:' \
        '--dir[working directory]:dir:_directories' \
        '--server[llama-server only]' \
        '--claude-bin[path to claude]:file:_files' \
        '--claude-dir[working directory for claude]:dir:_directories' \
        '--ready-timeout[readiness timeout]:duration:' \
        '--chat-template[chat template]:value:' \
        '--log-file[log file]:file:_files' \
        '--quiet-server[redirect server logs to file]' \
        '--log-max-mb[max log size before rotation]:n:' \
        '--log-keep[number of log archives]:n:' \
        '--webui-mcp-proxy[CORS proxy + mcp-search-server]' \
        '--mcp-addr[mcp-search-server address]:addr:' \
        '--mcp-config[path to MCP config]:file:_files' \
        '--mcp-strict[pass --strict-mcp-config]' \
        '--openai[OpenAI-compatible API URL]:url:' \
        '--key[external provider API key]:value:' \
        '--model-openai[external model name]:name:' \
        '--openai-path[path suffix]:value:' \
        '--proxy-addr[local proxy address]:addr:' \
        '--proxy-token[proxy auth token]:value:' \
        '--proxy-allow-remote[allow non-loopback proxy addr]' \
        '--config[config file path]:file:_files' \
        '-p[profile from config file]:name:' \
        '--doctor[diagnostics without spawning processes]' \
        '--update[update llama.cpp]' \
        '--llama-src[llama.cpp sources]:dir:_directories' \
        '--install-prefix[install prefix]:dir:_directories' \
        '--cuda[build with CUDA]' \
        '--cuda-arch[CUDA architecture]:value:' \
        '--version[print version and exit]' \
        '-v[verbose]' \
        '(-h --help)'{-h,--help}'[show help]'
}

_llama_runner "$@"