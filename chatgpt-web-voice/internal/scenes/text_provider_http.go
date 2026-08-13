package scenes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/config"
	"github.com/dyhhhhhh/chatgpt-web-voice/internal/scenes/prompts"
)

const maxTextResponseBytes = 1 << 20

// HTTPTextProvider is the OpenAI-compatible text orchestrator. It only calls
// POST /v1/chat/completions for candidate generation and SceneBrief
// composition. It uses VOICE_SCENE_AI_API_KEY and never reads, uses, or falls
// back to IMAGE_API_KEY or any account-pool token.
type HTTPTextProvider struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
	logger   *slog.Logger
}

// NewHTTPTextProvider builds the text orchestrator from scene text config.
func NewHTTPTextProvider(cfg config.SceneTextConfig, logger *slog.Logger) *HTTPTextProvider {
	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	endpoint, err := joinV1Endpoint(cfg.BaseURL, "/chat/completions")
	if err != nil {
		endpoint = ""
	}
	return &HTTPTextProvider{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    strings.TrimSpace(cfg.Model),
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// Name returns the provider family recorded on scene rows / logs.
func (p *HTTPTextProvider) Name() string { return TextProviderName }

// Model returns the configured text model name.
func (p *HTTPTextProvider) Model() string { return p.model }

// GenerateCandidates asks the text model for the strict candidate JSON.
func (p *HTTPTextProvider) GenerateCandidates(ctx context.Context, input CandidateInput) (CandidateResult, error) {
	var result CandidateResult
	userPrompt := buildCandidateUserPrompt(input)
	content, err := p.completeText(ctx, prompts.CandidateSystem(), userPrompt)
	if err != nil {
		return CandidateResult{}, err
	}
	raw, err := extractJSONContent(content)
	if err != nil {
		return CandidateResult{}, err
	}
	if err := strictDecodeJSON(raw, &result); err != nil {
		return CandidateResult{}, err
	}
	return result, nil
}

// ComposeBrief asks the text model for the strict SceneBrief JSON.
func (p *HTTPTextProvider) ComposeBrief(ctx context.Context, input BriefInput) (SceneBrief, error) {
	var brief SceneBrief
	userPrompt := buildBriefUserPrompt(input)
	content, err := p.completeText(ctx, prompts.BriefSystem(), userPrompt)
	if err != nil {
		return SceneBrief{}, err
	}
	raw, err := extractJSONContent(content)
	if err != nil {
		return SceneBrief{}, err
	}
	if err := strictDecodeJSON(raw, &brief); err != nil {
		return SceneBrief{}, err
	}
	return brief, nil
}

// completeText runs one chat completion with at most one transport retry.
// The request body is rebuilt on every attempt; the prompt is never logged.
func (p *HTTPTextProvider) completeText(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.endpoint == "" {
		return "", &ErrProviderResponse{Message: "text provider is misconfigured"}
	}
	payload := map[string]any{
		"model": p.model,
		"messages": []any{
			map[string]any{"role": "system", "content": systemPrompt},
			map[string]any{"role": "user", "content": userPrompt},
		},
		"response_format": map[string]any{"type": "json_object"},
		"temperature":     0.7,
	}

	for attempt := 1; attempt <= 2; attempt++ {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", &ErrProviderResponse{Message: "text request serialization failed"}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
		if err != nil {
			return "", &ErrProviderResponse{Message: "text request setup failed"}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if p.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}

		status, body, doErr := doProviderRequest(ctx, p.client, req, maxTextResponseBytes)
		if doErr != nil {
			// Transient network error: retry once while the context is alive.
			if attempt < 2 && ctx.Err() == nil && retryBackoff(ctx, attempt) {
				continue
			}
			return "", doErr
		}
		if status != http.StatusOK {
			if retryableStatus(status) && attempt < 2 && retryBackoff(ctx, attempt) {
				continue
			}
			return "", &ErrProviderResponse{Message: fmt.Sprintf("text provider returned HTTP %d", status)}
		}

		var response struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return "", &ErrProviderResponse{Message: "text provider returned invalid JSON"}
		}
		if response.Error != nil && len(response.Choices) == 0 {
			return "", &ErrProviderResponse{Message: "text provider rejected the request"}
		}
		if len(response.Choices) == 0 {
			return "", &ErrProviderResponse{Message: "text provider returned no choices"}
		}
		content := strings.TrimSpace(response.Choices[0].Message.Content)
		if content == "" {
			return "", &ErrProviderResponse{Message: "text provider returned an empty message"}
		}
		return content, nil
	}
	return "", &ErrProviderResponse{Message: "text provider request failed"}
}

// extractJSONContent strips Markdown code fences and surrounding prose from a
// model response, returning the outermost JSON object only. Compatibility with
// providers that wrap strict JSON in fences is handled here; structural
// validation always happens afterwards on the typed structs.
func extractJSONContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", &ErrProviderResponse{Message: "text model returned an empty response"}
	}
	if strings.HasPrefix(content, "```") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(content, "```"), "json")
		if idx := strings.Index(trimmed, "```"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		content = strings.TrimSpace(trimmed)
	}
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start < 0 || end <= start {
		return "", &ErrProviderResponse{Message: "text model response was not JSON"}
	}
	return content[start : end+1], nil
}

// strictDecodeJSON decodes one JSON payload and rejects unknown fields so a
// model drift cannot silently change the structured scene protocol.
func strictDecodeJSON(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &ErrProviderResponse{Message: "text model returned invalid scene JSON"}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &ErrProviderResponse{Message: "text model returned unexpected trailing JSON"}
	}
	return nil
}

// buildCandidateUserPrompt assembles the bounded conversation excerpt request.
func buildCandidateUserPrompt(input CandidateInput) string {
	var builder strings.Builder
	builder.WriteString("对话摘录如下。请根据这份摘录完成场景编排：\n\n")
	builder.WriteString("===== 对话摘录开始 =====\n")
	builder.WriteString(input.Excerpt)
	builder.WriteString("\n===== 对话摘录结束 =====\n\n")
	builder.WriteString("请严格按照系统规则返回 JSON。")
	return builder.String()
}

// buildBriefUserPrompt assembles the brief request from the approved summary
// and the selected candidate only.
func buildBriefUserPrompt(input BriefInput) string {
	var builder strings.Builder
	builder.WriteString("请根据以下已确认的内容编排短文与画面。\n\n")
	builder.WriteString("处境摘要：\n")
	builder.WriteString(input.ApprovedSummary)
	builder.WriteString("\n\n已选择的假如身份：\n")
	builder.WriteString(fmt.Sprintf("标题：%s\n", input.Candidate.Title))
	builder.WriteString(fmt.Sprintf("时刻：%s\n", input.Candidate.Moment))
	builder.WriteString(fmt.Sprintf("看得见的改变：%s\n", input.Candidate.VisibleChange))
	builder.WriteString(fmt.Sprintf("仍然存在的代价：%s\n", input.Candidate.RetainedCost))
	if len(input.Tensions) > 0 {
		builder.WriteString(fmt.Sprintf("张力参考：%s\n", strings.Join(input.Tensions, "、")))
	}
	if input.CultureLens != "" {
		builder.WriteString(fmt.Sprintf("文化视角参考：%s\n", input.CultureLens))
	}
	builder.WriteString("\n请严格按照系统规则返回 JSON。")
	return builder.String()
}
