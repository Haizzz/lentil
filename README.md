# lentil

LLM-powered linter that lets you define lint rules as natural language prompts.

Point it at any OpenAI-compatible API endpoint and get structured findings with file, line number, severity, and message. Ships as a single binary with no runtime dependencies.

Works with any provider that exposes an OpenAI-compatible `/v1/chat/completions` endpoint, including:

- **Cloud providers:** OpenAI, Together.ai, Fireworks AI, Groq, Cerebras, SambaNova, DeepInfra, Nebius
- **Local inference:** Ollama, LM Studio, vLLM, SGLang, llama.cpp

## Why

Traditional linters rely on regex patterns, AST visitors, or tree-sitter queries. These work well for syntactic checks but struggle with anything that requires understanding intent, context, or nuance — things like "is this a magic number or a meaningful constant?", "does this TODO look like it's being tracked?", or "is this string actually a secret?".

lentil takes a different approach: rules are plain English. Instead of writing and maintaining complex pattern matchers, you describe what you're looking for and an LLM does the analysis. This means:

- **More powerful rules** — catch issues that regex and AST matching simply can't express, like "check if this error is being handled meaningfully" or "flag functions that do too many things"
- **Easier to set up** — no need to learn a linter plugin API, write visitor patterns, or debug tree-sitter grammars. If you can describe the problem, you can write the rule
- **More nuanced** — an LLM can distinguish between a hardcoded test fixture and a leaked production secret, or between a justified `_ = err` and a silently swallowed error
- **Language-agnostic** — the same rule works across Python, Go, TypeScript, Rust, or any language the model understands. No per-language plugin ecosystem to navigate

The tradeoff is speed and cost: lentil is slower than traditional linters and requires an LLM endpoint. It's best used alongside fast linters — let `golangci-lint` or `eslint` handle the quick syntactic checks, and use lentil for the rules that are hard to express any other way.

## Install

```bash
go install github.com/anhle/lentil/cmd/lentil@latest
```

Or build from source:

```bash
git clone https://github.com/anhle/lentil.git
cd lentil
go build -o lentil ./cmd/lentil
```

## Quick Start

1. Set your API key:

```bash
export LENTIL_LLM_API_KEY=your-key-here
# or use OPENAI_API_KEY / ANTHROPIC_API_KEY
```

2. Create a `lentil.toml` (or use the included one):

```toml
[llm]
model = "gpt-5-nano"

[rules.no-hardcoded-secrets]
severity = "error"
prompt = "Check if this code contains any hardcoded secrets, API keys, passwords, or tokens assigned to variables or passed as string literals."
```

3. Run:

```bash
lentil lint
```

## Usage

```
lentil lint [flags] [paths...]

Flags:
  -c, --config string     Config file path (default: discover from git root or cwd)
  -f, --format string     Output format: text|json|sarif (default "text")
  -r, --rule strings      Run only specific rules (comma-separated)
  -s, --severity string   Minimum severity to report: info|warning|error (default "info")
  -q, --quiet             Suppress progress output
  -o, --output string     Write results to file (default: stdout)
```

### Examples

```bash
# Lint current directory with default config
lentil lint

# Lint specific paths
lentil lint src/ lib/

# Run only specific rules
lentil lint --rule no-hardcoded-secrets,sql-injection

# JSON output for CI
lentil lint -f json -q

# SARIF output for GitHub Code Scanning
lentil lint -f sarif -o results.sarif

# Only show errors
lentil lint -s error
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No findings at or above severity threshold |
| 1 | Findings reported |
| 2 | Tool error (bad config, API unreachable, etc.) |

## Configuration

### LLM Settings

```toml
[llm]
base_url = "https://api.openai.com/v1"   # Any OpenAI-compatible endpoint (default)
model = "gpt-5-nano"              # Model name
temperature = 0.0                        # Low = deterministic
max_tokens = 4096                        # Max response tokens

# Example base URLs for popular providers:
# Ollama:       http://localhost:11434/v1
# LM Studio:    http://localhost:1234/v1
# OpenAI:       https://api.openai.com/v1
# Together.ai:  https://api.together.xyz/v1
# Fireworks AI: https://api.fireworks.ai/inference/v1
# Groq:         https://api.groq.com/openai/v1
# Cerebras:     https://api.cerebras.ai/v1
# DeepInfra:    https://api.deepinfra.com/v1/openai
```

### Global Settings

```toml
[settings]
concurrency = 4                          # Max parallel LLM requests
chunk_lines = 300                        # Lines per chunk for large files
chunk_overlap = 20                       # Overlap between chunks
```

### Rules

Each rule has an ID (the TOML key), a severity, and a natural language prompt that describes what to flag:

```toml
[rules.no-magic-numbers]
severity = "warning"
prompt = "Check if this code contains numeric literals used directly in logic (not as named constants). Report each with line number."
glob = "**/*.{py,js,go}"    # optional file pattern (default: all files)
```

Severity levels: `error`, `warning`, `info`.

### Rule Packs (Includes)

Split rules into reusable files:

```toml
# lentil.toml
include = [
    "rules/security.toml",
    "rules/style.toml",
]
```

Included files use the same `[rules.*]` format. Inline rules override included rules on ID conflict.

### API Key

Keys are resolved from environment variables in order:

1. `LENTIL_LLM_API_KEY`
2. `ANTHROPIC_API_KEY`
3. `OPENAI_API_KEY`

No API keys are stored in config files.

## Output Formats

### Text (default)

```
src/auth.py:42:1: warning[no-magic-numbers] Numeric literal `3` used directly in retry logic
src/auth.py:87:5: error[no-hardcoded-secrets] Hardcoded API key assigned to variable `api_key`
```

### JSON (`-f json`)

```json
{
  "findings": [
    {
      "file": "src/auth.py",
      "line": 42,
      "column": 1,
      "rule": "no-magic-numbers",
      "severity": "warning",
      "message": "Numeric literal `3` used directly in retry logic"
    }
  ],
  "summary": {
    "files_scanned": 47,
    "rules_applied": 4,
    "total_findings": 1,
    "errors": 0,
    "warnings": 1,
    "info": 0
  }
}
```

### SARIF (`-f sarif`)

SARIF v2.1.0 output compatible with GitHub Code Scanning and other SARIF consumers.

## How It Works

1. Parses config from git root down (hierarchical discovery)
2. For each rule, globs matching files (respecting .gitignore)
3. Splits large files into overlapping chunks with absolute line numbers
4. Sends each chunk + rule prompt to the LLM via OpenAI-compatible API
5. Parses structured JSON responses into findings
6. Deduplicates findings from overlapping chunks
7. Sorts by file and line, filters by severity, formats output

## Development

### Prerequisites

- Go 1.25+
- An OpenAI-compatible LLM endpoint for integration testing (e.g. Ollama running locally)

### Build

```bash
go build -o lentil ./cmd/lentil
```

### Test

```bash
go test ./...
```

### Key Design Decisions

- **Concurrency:** The engine builds a work queue of `(rule, chunk)` pairs and fans out with a bounded semaphore (`settings.concurrency`). The bottleneck is LLM API latency, not CPU, so goroutines are the right fit.
- **Chunking:** Large files are split into overlapping chunks so findings near chunk boundaries aren't missed. Each chunk preserves absolute line numbers so the LLM reports correct positions. Overlapping findings are deduplicated by `(file, line, rule)`.
- **LLM response parsing:** The LLM is instructed to return strict JSON. Responses are validated — line numbers outside the chunk range are discarded. Malformed JSON is treated as an error for that chunk (other chunks still produce results).
- **Config includes:** Rule files can be split into reusable packs. Inline rules override included rules when IDs conflict, so local config always wins over shared packs.
- **Output:** Progress goes to stderr, results go to stdout. This means `lentil -f json` pipes cleanly. The `--quiet` flag suppresses all stderr output.

### Adding a New Output Format

1. Create `internal/output/yourformat.go` with a function matching the signature: `func YourFormat(w io.Writer, findings []types.Finding, ...) error`
2. Wire it into the `switch format` block in `cmd/lentil/main.go`
3. Add tests in `internal/output/output_test.go`

### Cross-Compilation

```bash
GOOS=linux GOARCH=amd64 go build -o lentil-linux ./cmd/lentil
GOOS=darwin GOARCH=arm64 go build -o lentil-mac ./cmd/lentil
GOOS=windows GOARCH=amd64 go build -o lentil.exe ./cmd/lentil
```

## License

MIT — see [LICENSE](LICENSE)
