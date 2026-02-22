package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anhle/lentil/internal/types"
)

func TestAnalyze_Success(t *testing.T) {
	llmResp := types.LLMResponse{
		Findings: []types.LLMFinding{
			{Line: 5, Column: 1, Message: "found issue", Snippet: "x = 42"},
		},
	}
	respJSON, _ := json.Marshal(llmResp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		chatResp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": string(respJSON)}},
			},
		}
		json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-model", "test-key", 0.0, 4096)

	rule := types.Rule{ID: "test", Severity: "warning", Prompt: "Find issues"}
	chunk := types.Chunk{
		FilePath:   "test.py",
		StartLine:  1,
		EndLine:    10,
		TotalLines: 10,
		Content:    " 1 | x = 42\n 2 | y = 0\n",
	}

	findings, err := client.Analyze(context.Background(), rule, chunk)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Line != 5 {
		t.Errorf("line = %d, want 5", findings[0].Line)
	}
	if findings[0].Message != "found issue" {
		t.Errorf("message = %q, want 'found issue'", findings[0].Message)
	}
}

func TestAnalyze_EmptyFindings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatResp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": `{"findings":[]}`}},
			},
		}
		json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "", 0.0, 4096)
	rule := types.Rule{ID: "test", Prompt: "test"}
	chunk := types.Chunk{FilePath: "f.py", StartLine: 1, EndLine: 5, TotalLines: 5, Content: "code"}

	findings, err := client.Analyze(context.Background(), rule, chunk)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestAnalyze_OutOfRangeLineFiltered(t *testing.T) {
	llmResp := types.LLMResponse{
		Findings: []types.LLMFinding{
			{Line: 3, Message: "in range"},
			{Line: 999, Message: "out of range"},
		},
	}
	respJSON, _ := json.Marshal(llmResp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatResp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": string(respJSON)}},
			},
		}
		json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "", 0.0, 4096)
	rule := types.Rule{ID: "test", Prompt: "test"}
	chunk := types.Chunk{FilePath: "f.py", StartLine: 1, EndLine: 10, TotalLines: 10, Content: "code"}

	findings, err := client.Analyze(context.Background(), rule, chunk)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (out of range filtered), got %d", len(findings))
	}
	if findings[0].Line != 3 {
		t.Errorf("expected line 3, got %d", findings[0].Line)
	}
}

func TestAnalyze_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatResp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "not valid json"}},
			},
		}
		json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "", 0.0, 4096)
	rule := types.Rule{ID: "test", Prompt: "test"}
	chunk := types.Chunk{FilePath: "f.py", StartLine: 1, EndLine: 5, TotalLines: 5, Content: "code"}

	_, err := client.Analyze(context.Background(), rule, chunk)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestAnalyze_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "", 0.0, 4096)
	rule := types.Rule{ID: "test", Prompt: "test"}
	chunk := types.Chunk{FilePath: "f.py", StartLine: 1, EndLine: 5, TotalLines: 5, Content: "code"}

	_, err := client.Analyze(context.Background(), rule, chunk)
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
}

func TestAnalyze_NoAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("should not send Authorization header when no API key")
		}
		chatResp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": `{"findings":[]}`}},
			},
		}
		json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "m", "", 0.0, 4096)
	rule := types.Rule{ID: "test", Prompt: "test"}
	chunk := types.Chunk{FilePath: "f.py", StartLine: 1, EndLine: 5, TotalLines: 5, Content: "code"}

	_, err := client.Analyze(context.Background(), rule, chunk)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
}
