package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ErrAssistantDisabled is returned when the voice assistant is not enabled in
// the group's settings. Handlers map it to 409.
var ErrAssistantDisabled = errors.New("voice assistant is not enabled for this group")

// ErrAssistantSTTNotConfigured is returned when the voice assistant is
// enabled but the group's speech-to-text settings are incomplete. Handlers
// map it to 409.
var ErrAssistantSTTNotConfigured = errors.New("voice assistant speech-to-text is not fully configured (stt_base_url, stt_api_key, stt_model)")

const (
	// AssistantMaxAudioBytes caps the audio payload accepted by the voice
	// assistant endpoint.
	AssistantMaxAudioBytes = 10 << 20

	// assistantMaxTextLen caps the transcribed text accepted by
	// ParseAssistantCommand.
	assistantMaxTextLen = 2000

	// assistantMaxHistoryMessages bounds how many conversation turns are
	// forwarded to the model so a long session cannot blow up the token count.
	assistantMaxHistoryMessages = 10

	// assistantMaxActions caps how many action proposals a single reply
	// returns.
	assistantMaxActions = 10
)

// Assistant action types proposed by the model. The backend never executes
// them; the frontend asks the user for confirmation and calls the existing
// APIs itself (same pattern as ai-batch-parse).
const (
	AssistantActionCreateLocation = "create_location"
	AssistantActionCreateItem     = "create_item"
	AssistantActionQueryItem      = "query_item"
	AssistantActionQueryLocation  = "query_location"
)

// STTConfig holds the group's speech-to-text settings, stored in the group
// settings document under the "assistant" namespace.
type STTConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// AssistantMessage is one turn of the voice-assistant conversation history
// kept client-side and re-sent with every request.
type AssistantMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AssistantAction is a single action proposal in an AssistantReply. Only the
// fields relevant to the action's Type are populated.
type AssistantAction struct {
	Type         string   `json:"type"`
	Name         string   `json:"name,omitempty"`
	ParentPath   *string  `json:"parent_path,omitempty"`
	LocationPath *string  `json:"location_path,omitempty"`
	Quantity     *float64 `json:"quantity,omitempty"`
	Description  *string  `json:"description,omitempty"`
	// Expiry fields for create_item: dates as YYYY-MM-DD, shelf life in days.
	ProductionDate string `json:"production_date,omitempty"`
	ShelfLifeDays  *int   `json:"shelf_life_days,omitempty"`
	ExpiryDate     string `json:"expiry_date,omitempty"`
	Keyword        string `json:"keyword,omitempty"`
}

// AssistantReply is the parsed model response: a natural-language reply for
// the user plus zero or more action proposals.
type AssistantReply struct {
	Reply   string            `json:"reply"`
	Actions []AssistantAction `json:"actions"`
}

// AssistantSTTConfig extracts the voice-assistant STT configuration from the
// group's settings document, returning ErrAssistantDisabled or
// ErrAssistantSTTNotConfigured when the feature is off or incompletely set up.
func AssistantSTTConfig(settings map[string]interface{}) (STTConfig, error) {
	ns, ok := settings[settingsNamespaceAssistant].(map[string]interface{})
	if !ok {
		return STTConfig{}, ErrAssistantDisabled
	}

	enabled, _ := ns[settingsKeyAssistantEnabled].(bool)
	if !enabled {
		return STTConfig{}, ErrAssistantDisabled
	}

	cfg := STTConfig{
		BaseURL: strings.TrimSpace(stringSetting(ns, settingsKeySTTBaseURL)),
		APIKey:  strings.TrimSpace(stringSetting(ns, settingsKeySTTAPIKey)),
		Model:   strings.TrimSpace(stringSetting(ns, settingsKeySTTModel)),
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return STTConfig{}, ErrAssistantSTTNotConfigured
	}

	return cfg, nil
}

func stringSetting(ns map[string]interface{}, key string) string {
	v, _ := ns[key].(string)
	return v
}

// TranscribeAudio sends a recorded audio clip to an OpenAI-compatible
// speech-to-text endpoint (POST {base}/audio/transcriptions) and returns the
// transcript.
func (svc *AIService) TranscribeAudio(ctx context.Context, audio []byte, mimeType string, sttCfg STTConfig) (string, error) {
	_, span := entityServiceTracer().Start(ctx, "service.AIService.TranscribeAudio",
		trace.WithAttributes(
			attribute.String("audio.mime_type", mimeType),
			attribute.Int("audio.size", len(audio)),
			attribute.String("stt.model", sttCfg.Model),
		))
	defer span.End()

	if len(audio) == 0 {
		err := errors.New("audio is empty")
		recordServiceSpanError(span, err)
		return "", err
	}
	if len(audio) > AssistantMaxAudioBytes {
		err := fmt.Errorf("audio exceeds %d bytes", AssistantMaxAudioBytes)
		recordServiceSpanError(span, err)
		return "", err
	}
	if sttCfg.BaseURL == "" || sttCfg.APIKey == "" || sttCfg.Model == "" {
		err := ErrAssistantSTTNotConfigured
		recordServiceSpanError(span, err)
		return "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "audio."+assistantAudioExtension(mimeType))
	if err != nil {
		return "", fmt.Errorf("building stt request: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", fmt.Errorf("building stt request: %w", err)
	}
	if err := writer.WriteField("model", sttCfg.Model); err != nil {
		return "", fmt.Errorf("building stt request: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("building stt request: %w", err)
	}

	endpoint := strings.TrimSuffix(sttCfg.BaseURL, "/") + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("building stt request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+sttCfg.APIKey)

	resp, err := svc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stt request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading stt response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("stt endpoint returned status %d: %s", resp.StatusCode, truncateAIString(string(respBody), aiErrorBodySnippet))
	}

	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("decoding stt response: %w", err)
	}

	text := strings.TrimSpace(parsed.Text)
	if text == "" {
		err := errors.New("stt endpoint returned an empty transcript")
		recordServiceSpanError(span, err)
		return "", err
	}

	return text, nil
}

// assistantAudioExtension maps an audio MIME type to a filename extension the
// STT endpoint can route on; unknown types default to webm (what browsers
// record with MediaRecorder).
func assistantAudioExtension(mimeType string) string {
	base, _, _ := strings.Cut(mimeType, ";")
	switch strings.ToLower(strings.TrimSpace(base)) {
	case "audio/ogg", "application/ogg":
		return "ogg"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return "wav"
	case "audio/mp4", "audio/x-m4a", "audio/m4a":
		return "m4a"
	default:
		return "webm"
	}
}

// ParseAssistantCommand interprets one transcribed voice command with the
// configured chat model, returning the assistant's natural-language reply and
// any action proposals (create location/item, query item/location). The
// conversation history is truncated to the most recent turns. A model reply
// that is not valid protocol JSON degrades to a plain-text reply with no
// actions instead of an error.
func (svc *AIService) ParseAssistantCommand(ctx context.Context, groupID uuid.UUID, history []AssistantMessage, text string) (AssistantReply, error) {
	spanCtx, span := entityServiceTracer().Start(ctx, "service.AIService.ParseAssistantCommand",
		trace.WithAttributes(
			attribute.String("group.id", groupID.String()),
			attribute.String("ai.model", svc.config.Model),
			attribute.Int("text.length", len(text)),
			attribute.Int("history.length", len(history)),
		))
	defer span.End()
	ctx = spanCtx

	if !svc.config.Enabled || svc.config.APIKey == "" {
		return AssistantReply{}, ErrAIDisabled
	}

	text = strings.TrimSpace(text)
	if text == "" {
		err := errors.New("assistant text is empty")
		recordServiceSpanError(span, err)
		return AssistantReply{}, err
	}
	if len(text) > assistantMaxTextLen {
		err := fmt.Errorf("assistant text exceeds %d characters", assistantMaxTextLen)
		recordServiceSpanError(span, err)
		return AssistantReply{}, err
	}

	candidates, err := svc.loadCandidates(ctx, groupID)
	if err != nil {
		recordServiceSpanError(span, err)
		return AssistantReply{}, err
	}

	messages := []aiChatMessage{
		{Role: aiChatRoleSystem, Content: buildAIAssistantPrompt(candidates.Locations)},
	}

	if len(history) > assistantMaxHistoryMessages {
		history = history[len(history)-assistantMaxHistoryMessages:]
	}
	for _, msg := range history {
		if msg.Role != aiChatRoleUser && msg.Role != aiChatRoleAssistant {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		messages = append(messages, aiChatMessage{Role: msg.Role, Content: content})
	}
	messages = append(messages, aiChatMessage{Role: aiChatRoleUser, Content: text})

	content, err := svc.chat(ctx, messages)
	if err != nil {
		recordServiceSpanError(span, err)
		return AssistantReply{}, err
	}

	return parseAIAssistantReply(content), nil
}

// buildAIAssistantPrompt renders the system prompt for the voice assistant,
// constraining location paths to the group's existing location tree.
func buildAIAssistantPrompt(locations []aiCandidate) string {
	var sb strings.Builder

	sb.WriteString(`你是家庭库存管理应用 Homebox 的语音助手。用户通过语音与你对话（中文或英文都可能），帮助你管理物品和位置。
你只返回一个 JSON 对象（不要 markdown、不要代码块、不要任何多余文字），格式如下：
{"reply": string, "actions": [ ... ]}

字段说明：
- "reply"：你给用户的自然语言回复，使用与用户输入相同的语言。例如「我找到了 002 位置，要在这里新建风扇物品吗？」
- "actions"：动作提案数组；纯闲聊或需要向用户追问澄清时使用空数组 []。这些动作不会立即执行，前端会展示确认卡片，用户确认后才调用现有 API 执行。

动作类型（type 字段）：
1. create_location — 新建位置：
   {"type": "create_location", "name": string, "parent_path": string|null}
   - parent_path 为父位置的完整路径，null 表示顶级位置；只能从下方候选位置路径中选择。
2. create_item — 新建物品：
   {"type": "create_item", "name": string, "location_path": string|null, "quantity": number|null, "description": string|null, "production_date": string|null, "shelf_life_days": number|null, "expiry_date": string|null}
   - location_path 必须完全取自下方候选位置路径，用户未指明位置时用 null。
   - quantity 未指明时用 null，description 未指明时用 null。
   - 生产日期用 production_date，截止日期用 expiry_date，格式均为 YYYY-MM-DD（月和日必须补零，如 2026-07-01，不要写成 2026-7-1），未指明时省略。
   - 保质期用 shelf_life_days（整数天数）；用户说“12个月”“一年”等时长时换算成天数（一年按 365 天、一个月按 30 天）。
3. query_item — 查询物品：{"type": "query_item", "keyword": string}
4. query_location — 查询位置：{"type": "query_location", "keyword": string}

规则：
- 用户意图含糊时优先在 reply 中追问澄清，actions 使用空数组。
- 不要发明候选列表中不存在的位置路径。
- 一轮对话可以提出多个动作（例如「在 002 新建一个位置并放入风扇」）。

候选位置（完整路径）：
`)
	if len(locations) == 0 {
		sb.WriteString("(无)\n")
	}
	for _, l := range locations {
		fmt.Fprintf(&sb, "- %s\n", l.Path)
	}

	return sb.String()
}

// parseAIAssistantReply decodes the model reply into an AssistantReply.
// Unknown action types and actions missing their required fields are dropped;
// a reply that contains no usable JSON at all degrades to the raw text with
// no actions.
func parseAIAssistantReply(content string) AssistantReply {
	raw := extractJSONObject(content)
	if raw == "" {
		return AssistantReply{Reply: strings.TrimSpace(content), Actions: []AssistantAction{}}
	}

	var parsed struct {
		Reply   string            `json:"reply"`
		Actions []AssistantAction `json:"actions"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return AssistantReply{Reply: strings.TrimSpace(content), Actions: []AssistantAction{}}
	}

	out := AssistantReply{
		Reply:   strings.TrimSpace(parsed.Reply),
		Actions: []AssistantAction{},
	}

	for _, action := range parsed.Actions {
		action.Name = strings.TrimSpace(action.Name)
		action.Keyword = strings.TrimSpace(action.Keyword)

		switch action.Type {
		case AssistantActionCreateLocation, AssistantActionCreateItem:
			if action.Name == "" {
				continue
			}
		case AssistantActionQueryItem, AssistantActionQueryLocation:
			if action.Keyword == "" {
				continue
			}
		default:
			continue
		}

		if action.Quantity != nil && *action.Quantity < 1 {
			action.Quantity = nil
		}

		// Normalize AI-spoken dates to YYYY-MM-DD; unparseable values are
		// dropped rather than passed through to the create form.
		if action.ProductionDate != "" {
			action.ProductionDate = validAIDate(&action.ProductionDate)
		}
		if action.ExpiryDate != "" {
			action.ExpiryDate = validAIDate(&action.ExpiryDate)
		}

		out.Actions = append(out.Actions, action)
		if len(out.Actions) >= assistantMaxActions {
			break
		}
	}

	return out
}
