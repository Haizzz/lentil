<p align="center">
  <img src="lentil.png" alt="lentil" width="200">
</p>

# lentil

LLM-powered linter that takes lint rules as natural language prompts. It sends code to any OpenAI-compatible endpoint and returns structured findings with file, line number, severity, and message. Ships as a single binary with no runtime dependencies.

Works with anything that exposes `/v1/chat/completions`: OpenAI, Ollama, LM Studio, Together.ai, Groq, vLLM, llama.cpp, and others.

## Why

Traditional linters rely on regex, AST visitors, or tree-sitter queries. These handle syntactic checks well, but they struggle with anything that requires understanding context or intent. Questions like "is this a magic number or a meaningful constant?", "is this `_ = err` justified?", or "is that string a test fixture or a leaked secret?" don't have clean pattern-matching answers.

lentil takes rules as natural language prompts and uses an LLM to do the analysis. The same rule works across Python, Go, TypeScript, or whatever the model understands, and you can write a new rule in a sentence instead of building a visitor pattern or debugging a tree-sitter grammar.

It's slower than traditional linters and requires an LLM endpoint, so it works best alongside tools like `golangci-lint` or `eslint` rather than replacing them.

## Install

Download a prebuilt binary from the [latest release](https://github.com/Haizzz/lentil/releases/latest), or install with Go:

```bash
go install github.com/Haizzz/lentil/cmd/lentil@latest
```

Or build from source:

```bash
git clone https://github.com/Haizzz/lentil.git
cd lentil
go build -o lentil ./cmd/lentil
```

## Quick start

1. Set your API key:

```bash
export LENTIL_LLM_API_KEY=your-key-here
# also checks OPENAI_API_KEY and ANTHROPIC_API_KEY
```

2. Create a `lentil.toml`:

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

Findings print to stdout in the same format as other linters.

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

```bash
# Lint specific paths
lentil lint src/ lib/

# Run a subset of rules
lentil lint --rule no-hardcoded-secrets,sql-injection

# JSON output for CI
lentil lint -f json -q

# SARIF for GitHub Code Scanning
lentil lint -f sarif -o results.sarif

# Only errors
lentil lint -s error
```

Exit codes: `0` clean, `1` findings reported, `2` tool error (bad config, API unreachable, etc.).

## Configuration

### LLM

```toml
[llm]
base_url = "https://api.openai.com/v1"   # any OpenAI-compatible endpoint
model = "gpt-5-nano"
temperature = 0.0                        # low = deterministic
max_tokens = 4096
```

Common base URLs:

- Ollama: `http://localhost:11434/v1`
- LM Studio: `http://localhost:1234/v1`
- Together.ai: `https://api.together.xyz/v1`
- Groq: `https://api.groq.com/openai/v1`
- Fireworks AI: `https://api.fireworks.ai/inference/v1`
- Cerebras: `https://api.cerebras.ai/v1`
- DeepInfra: `https://api.deepinfra.com/v1/openai`

### Settings

```toml
[settings]
concurrency = 4      # max parallel LLM requests
chunk_lines = 300    # lines per chunk for large files
chunk_overlap = 20   # overlap between chunks
```

### Rules

Each rule has an ID (the TOML key), a severity, and a prompt describing what to flag:

```toml
[rules.no-magic-numbers]
severity = "warning"
prompt = "Check if this code contains numeric literals used directly in logic (not as named constants). Report each with line number."
glob = "**/*.{py,js,go}"    # optional, defaults to all files
```

Severity levels: `error`, `warning`, `info`.

### API key

lentil resolves API keys from environment variables in this order: `LENTIL_LLM_API_KEY`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`. Keys are not stored in config files.

## Output formats

**Text** (default):

```
src/auth.py:42:1: warning[no-magic-numbers] Numeric literal `3` used directly in retry logic
src/auth.py:87:5: error[no-hardcoded-secrets] Hardcoded API key assigned to variable `api_key`
```

**JSON** (`-f json`):

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

**SARIF** (`-f sarif`): v2.1.0 output compatible with GitHub Code Scanning.

## How it works

lentil discovers config starting from the git root. For each rule, it globs matching files while respecting `.gitignore`, then splits large files into overlapping chunks that preserve absolute line numbers. Each chunk gets sent to the LLM alongside the rule prompt. The LLM returns structured JSON, which lentil validates, deduplicates across chunk boundaries, sorts by file and line, filters by severity, and formats for output.

Progress goes to stderr, results to stdout, so `lentil -f json` pipes cleanly.

## Development

Requires Go 1.25+ and an OpenAI-compatible endpoint for integration tests (Ollama locally works fine).

```bash
go build -o lentil ./cmd/lentil
go test ./...
```

Tagged releases are cross-compiled via [GoReleaser](https://goreleaser.com/) (see `.goreleaser.yml`).

## Licence

MIT. See [LICENSE](LICENSE).
