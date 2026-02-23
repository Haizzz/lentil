package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/anhle/lentil/internal/lint"
)

const systemPrompt = `You are a code linter. Analyze the provided source code against the given rule.
For each violation found, report the line number, column if known, a description of the issue, and the offending code snippet.
If the code follows the rule, return an empty findings array.
Respond in JSON.`

var findingsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"findings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"line":    map[string]any{"type": "integer"},
					"column":  map[string]any{"type": "integer"},
					"message": map[string]any{"type": "string"},
					"snippet": map[string]any{"type": "string"},
				},
				"required":             []string{"line", "column", "message", "snippet"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"findings"},
	"additionalProperties": false,
}

type llmResponse struct {
	Findings []Finding `json:"findings"`
}

type Finding struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Snippet string `json:"snippet"`
}

// Client wraps the official OpenAI Go SDK. Works with any OpenAI-compatible
// provider via a custom base URL.
type Client struct {
	api       *openai.Client
	model     string
	temp      float64
	maxTokens int
}

func NewClient(baseURL, model, apiKey string, temp float64, maxTokens int) *Client {
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(3),
		option.WithRequestTimeout(60 * time.Second),
	}
	opts = append(opts, option.WithAPIKey(apiKey))

	client := openai.NewClient(opts...)

	return &Client{
		api:       &client,
		model:     model,
		temp:      temp,
		maxTokens: maxTokens,
	}
}

// Analyze sends a rule + chunk to the LLM and returns parsed findings.
func (c *Client) Analyze(ctx context.Context, rule lint.Rule, chunk lint.Chunk) ([]Finding, error) {
	userPrompt := buildUserPrompt(rule, chunk)

	completion, err := c.api.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:       openai.ChatModel(c.model),
		Temperature: openai.Float(c.temp),
		MaxCompletionTokens: openai.Int(int64(c.maxTokens)),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "findings",
					Schema: findingsSchema,
					Strict: openai.Bool(true),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("chat completion: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("API returned no choices")
	}

	content := completion.Choices[0].Message.Content
	var result llmResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parsing LLM JSON response: %w", err)
	}

	var valid []Finding
	for _, f := range result.Findings {
		if f.Line >= chunk.StartLine && f.Line <= chunk.EndLine {
			valid = append(valid, f)
		}
	}

	return valid, nil
}

func buildUserPrompt(rule lint.Rule, chunk lint.Chunk) string {
	header := fmt.Sprintf("Rule: %s\n\nFile: %s", rule.Prompt, chunk.FilePath)
	if chunk.TotalLines > chunk.EndLine-chunk.StartLine+1 {
		header += fmt.Sprintf(" (lines %d-%d of %d)", chunk.StartLine, chunk.EndLine, chunk.TotalLines)
	}

	return header + "\n" + chunk.Content
}
