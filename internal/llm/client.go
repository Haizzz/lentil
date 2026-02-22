package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/anhle/lentil/internal/types"
)

const systemPrompt = `You are a code linter. Analyze the provided source code against the given rule.
Respond ONLY with a JSON object in this exact format:
{
  "findings": [
    {
      "line": <line_number>,
      "column": <column_number_or_null>,
      "message": "<description of the issue>",
      "snippet": "<the offending code snippet>"
    }
  ]
}
If there are no findings, return: {"findings": []}
Do not include any text outside the JSON object.`

// Client is an OpenAI-compatible /v1/chat/completions API client.
// Works with OpenAI, Together.ai, Fireworks AI, Groq, Cerebras, SambaNova,
// DeepInfra, Ollama, LM Studio, vLLM, SGLang, and any other provider that
// implements the OpenAI chat completions interface.
type Client struct {
	baseURL    string
	model      string
	apiKey     string
	temp       float64
	maxTokens  int
	httpClient *http.Client
}

// NewClient creates a new LLM client.
func NewClient(baseURL, model, apiKey string, temp float64, maxTokens int) *Client {
	return &Client{
		baseURL:   baseURL,
		model:     model,
		apiKey:    apiKey,
		temp:      temp,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Analyze sends a rule + chunk to the LLM and returns parsed findings.
func (c *Client) Analyze(ctx context.Context, rule types.Rule, chunk types.Chunk) ([]types.LLMFinding, error) {
	userPrompt := buildUserPrompt(rule, chunk)

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: c.temp,
		MaxTokens:   c.maxTokens,
	}

	var result types.LLMResponse

	operation := func() error {
		body, err := json.Marshal(reqBody)
		if err != nil {
			return backoff.Permanent(fmt.Errorf("marshaling request: %w", err))
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("creating request: %w", err))
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("sending request: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		// Retry on rate limit or server errors
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
		}
		if resp.StatusCode != 200 {
			return backoff.Permanent(fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody)))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return backoff.Permanent(fmt.Errorf("parsing API response: %w", err))
		}

		if chatResp.Error != nil {
			return backoff.Permanent(fmt.Errorf("API error: %s", chatResp.Error.Message))
		}

		if len(chatResp.Choices) == 0 {
			return backoff.Permanent(fmt.Errorf("API returned no choices"))
		}

		content := chatResp.Choices[0].Message.Content
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			return backoff.Permanent(fmt.Errorf("parsing LLM JSON response: %w (content: %s)", err, truncate(content, 200)))
		}

		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 3 * time.Minute
	bo.InitialInterval = 2 * time.Second
	bCtx := backoff.WithContext(backoff.WithMaxRetries(bo, 3), ctx)

	if err := backoff.Retry(operation, bCtx); err != nil {
		return nil, err
	}

	// Validate line numbers are in range
	var valid []types.LLMFinding
	for _, f := range result.Findings {
		if f.Line >= chunk.StartLine && f.Line <= chunk.EndLine {
			valid = append(valid, f)
		}
	}

	return valid, nil
}

func buildUserPrompt(rule types.Rule, chunk types.Chunk) string {
	header := fmt.Sprintf("Rule: %s\n\nFile: %s", rule.Prompt, chunk.FilePath)
	if chunk.TotalLines > chunk.EndLine-chunk.StartLine+1 {
		header += fmt.Sprintf(" (lines %d-%d of %d)", chunk.StartLine, chunk.EndLine, chunk.TotalLines)
	}
	return header + "\n" + chunk.Content
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
