package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ErrAIDisabled is returned when the AI photo-recognition integration is not
// enabled or has no API key configured. Handlers map it to 503.
var ErrAIDisabled = errors.New("ai photo recognition is not enabled")

const (
	// aiDefaultTimeout guards against a zero/unset configured timeout.
	aiDefaultTimeout = 30 * time.Second

	// aiMaxResponseBytes caps how much of the AI endpoint's response body is
	// read, so a misbehaving endpoint cannot exhaust memory.
	aiMaxResponseBytes = 1 << 20

	// aiErrorBodySnippet is how much of a non-2xx response body is included
	// in the returned error.
	aiErrorBodySnippet = 256
)

type AIService struct {
	repos  *repo.AllRepos
	config config.AIConf
	client *http.Client
}

func NewAIService(repos *repo.AllRepos, cfg config.AIConf) *AIService {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = aiDefaultTimeout
	}

	return &AIService{
		repos:  repos,
		config: cfg,
		client: &http.Client{Timeout: timeout},
	}
}

// EntityAISuggestion is the AI-proposed draft for a new entity, derived from
// a photo. SuggestedTagIDs/SuggestedLocationID only ever contain IDs that
// exist in the acting user's group.
type EntityAISuggestion struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Quantity            float64  `json:"quantity"`
	SuggestedTagIDs     []string `json:"suggestedTagIds"`
	SuggestedLocationID *string  `json:"suggestedLocationId"`
	Confidence          float64  `json:"confidence"`
}

// aiCandidate is a single tag or location option presented to the model.
type aiCandidate struct {
	ID   string
	Name string
	Path string
}

type aiCandidates struct {
	Tags      []aiCandidate
	Locations []aiCandidate
}

type (
	aiChatRequest struct {
		Model    string          `json:"model"`
		Messages []aiChatMessage `json:"messages"`
	}

	aiChatMessage struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	aiContentPart struct {
		Type     string      `json:"type"`
		Text     string      `json:"text,omitempty"`
		ImageURL *aiImageURL `json:"image_url,omitempty"`
	}

	aiImageURL struct {
		URL string `json:"url"`
	}

	aiChatResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	aiSuggestionPayload struct {
		Name                string   `json:"name"`
		Description         string   `json:"description"`
		Quantity            *float64 `json:"quantity"`
		SuggestedTagIDs     []string `json:"suggestedTagIds"`
		SuggestedLocationID *string  `json:"suggestedLocationId"`
		Confidence          *float64 `json:"confidence"`
	}
)

// SuggestFromPhoto analyzes a photo of a household item with the configured
// OpenAI-compatible vision model and returns a suggested entity draft. Tag
// and location suggestions are constrained to the group's existing tags and
// locations.
func (svc *AIService) SuggestFromPhoto(ctx context.Context, groupID uuid.UUID, image []byte, mimeType string) (EntityAISuggestion, error) {
	spanCtx, span := entityServiceTracer().Start(ctx, "service.AIService.SuggestFromPhoto",
		trace.WithAttributes(
			attribute.String("group.id", groupID.String()),
			attribute.String("ai.model", svc.config.Model),
			attribute.String("image.mime_type", mimeType),
			attribute.Int("image.size", len(image)),
		))
	defer span.End()
	ctx = spanCtx

	if !svc.config.Enabled || svc.config.APIKey == "" {
		return EntityAISuggestion{}, ErrAIDisabled
	}

	if len(image) == 0 {
		err := errors.New("image is empty")
		recordServiceSpanError(span, err)
		return EntityAISuggestion{}, err
	}

	candidates, err := svc.loadCandidates(ctx, groupID)
	if err != nil {
		recordServiceSpanError(span, err)
		return EntityAISuggestion{}, err
	}

	suggestion, err := svc.suggest(ctx, candidates, image, mimeType)
	if err != nil {
		recordServiceSpanError(span, err)
		return EntityAISuggestion{}, err
	}

	return suggestion, nil
}

// loadCandidates builds the tag/location option lists for the group. Location
// entries carry their full hierarchy path (e.g. "Home / Garage / Shelf").
func (svc *AIService) loadCandidates(ctx context.Context, gid uuid.UUID) (aiCandidates, error) {
	tags, err := svc.repos.Tags.GetAll(ctx, gid)
	if err != nil {
		return aiCandidates{}, err
	}

	tree, err := svc.repos.Entities.Tree(ctx, gid, repo.TreeQuery{WithItems: false})
	if err != nil {
		return aiCandidates{}, err
	}

	out := aiCandidates{
		Tags:      make([]aiCandidate, 0, len(tags)),
		Locations: []aiCandidate{},
	}

	for _, t := range tags {
		out.Tags = append(out.Tags, aiCandidate{ID: t.ID.String(), Name: t.Name})
	}

	var walk func(items []*repo.TreeItem, prefix string)
	walk = func(items []*repo.TreeItem, prefix string) {
		for _, item := range items {
			path := item.Name
			if prefix != "" {
				path = prefix + " / " + item.Name
			}
			out.Locations = append(out.Locations, aiCandidate{ID: item.ID.String(), Name: item.Name, Path: path})
			walk(item.Children, path)
		}
	}

	top := make([]*repo.TreeItem, len(tree))
	for i := range tree {
		top[i] = &tree[i]
	}
	walk(top, "")

	return out, nil
}

func (svc *AIService) suggest(ctx context.Context, candidates aiCandidates, image []byte, mimeType string) (EntityAISuggestion, error) {
	payload, err := json.Marshal(aiChatRequest{
		Model: svc.config.Model,
		Messages: []aiChatMessage{
			{Role: "system", Content: buildAISystemPrompt(candidates, svc.config.Language)},
			{Role: "user", Content: []aiContentPart{
				{Type: "text", Text: "Analyze this photo of a household item and reply with the JSON object described in the system instructions."},
				{Type: "image_url", ImageURL: &aiImageURL{URL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)}},
			}},
		},
	})
	if err != nil {
		return EntityAISuggestion{}, fmt.Errorf("marshaling ai request: %w", err)
	}

	endpoint := strings.TrimSuffix(svc.config.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return EntityAISuggestion{}, fmt.Errorf("building ai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+svc.config.APIKey)

	resp, err := svc.client.Do(req)
	if err != nil {
		return EntityAISuggestion{}, fmt.Errorf("ai request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxResponseBytes))
	if err != nil {
		return EntityAISuggestion{}, fmt.Errorf("reading ai response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet := string(body)
		if len(snippet) > aiErrorBodySnippet {
			snippet = snippet[:aiErrorBodySnippet] + "..."
		}
		return EntityAISuggestion{}, fmt.Errorf("ai endpoint returned status %d: %s", resp.StatusCode, snippet)
	}

	var chatResp aiChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return EntityAISuggestion{}, fmt.Errorf("decoding ai response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return EntityAISuggestion{}, errors.New("ai endpoint returned no choices")
	}

	return parseAISuggestion(chatResp.Choices[0].Message.Content, candidates)
}

// buildAISystemPrompt renders the instruction prompt including the candidate
// tag/location lists the model must restrict its suggestions to. language is
// the natural language the suggested name/description must be written in.
func buildAISystemPrompt(candidates aiCandidates, language string) string {
	if language == "" {
		language = "English"
	}

	var sb strings.Builder

	sb.WriteString(`You are analyzing a photo of a household item for a home-inventory app.
Reply with ONLY a JSON object (no markdown, no prose, no code fences) of this exact shape:
{"name": string, "description": string, "quantity": number, "suggestedTagIds": [string], "suggestedLocationId": string|null, "confidence": number}

Rules:
- "name" is a short item name; "description" is one or two sentences about the item.
- "name" and "description" must be written in ` + language + `.
- "quantity" is the number of items visible; use 1 when unsure; never below 1.
- "suggestedTagIds" must be chosen ONLY from the candidate tag ids below (use [] if none fit).
- "suggestedLocationId" must be chosen ONLY from the candidate location ids below, or null if none fit.
- "confidence" is your confidence in the overall suggestion, between 0 and 1.

Candidate tags (id: name):
`)
	if len(candidates.Tags) == 0 {
		sb.WriteString("(none)\n")
	}
	for _, t := range candidates.Tags {
		fmt.Fprintf(&sb, "- %s: %s\n", t.ID, t.Name)
	}

	sb.WriteString("\nCandidate locations (id: name (path)):\n")
	if len(candidates.Locations) == 0 {
		sb.WriteString("(none)\n")
	}
	for _, l := range candidates.Locations {
		fmt.Fprintf(&sb, "- %s: %s (path: %s)\n", l.ID, l.Name, l.Path)
	}

	return sb.String()
}

// extractJSONObject leniently extracts the first complete {...} block from a
// model reply, tolerating leading/trailing prose and ```json code fences.
func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")

	start := strings.Index(content, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}

	return ""
}

// parseAISuggestion decodes the model reply and validates it against the
// candidate lists: unknown tag ids are dropped, an unknown location id is
// nulled out, quantity is clamped to >= 1 and confidence to [0, 1].
func parseAISuggestion(content string, candidates aiCandidates) (EntityAISuggestion, error) {
	raw := extractJSONObject(content)
	if raw == "" {
		return EntityAISuggestion{}, fmt.Errorf("no JSON object found in ai reply: %q", truncateAIString(content, aiErrorBodySnippet))
	}

	var parsed aiSuggestionPayload
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return EntityAISuggestion{}, fmt.Errorf("decoding ai suggestion: %w", err)
	}

	knownTags := make(map[string]struct{}, len(candidates.Tags))
	for _, t := range candidates.Tags {
		knownTags[t.ID] = struct{}{}
	}

	knownLocations := make(map[string]struct{}, len(candidates.Locations))
	for _, l := range candidates.Locations {
		knownLocations[l.ID] = struct{}{}
	}

	suggestion := EntityAISuggestion{
		Name:            strings.TrimSpace(parsed.Name),
		Description:     strings.TrimSpace(parsed.Description),
		Quantity:        1,
		SuggestedTagIDs: []string{},
	}

	for _, id := range parsed.SuggestedTagIDs {
		if _, ok := knownTags[id]; ok {
			suggestion.SuggestedTagIDs = append(suggestion.SuggestedTagIDs, id)
		}
	}

	if parsed.SuggestedLocationID != nil {
		if _, ok := knownLocations[*parsed.SuggestedLocationID]; ok {
			suggestion.SuggestedLocationID = parsed.SuggestedLocationID
		}
	}

	if parsed.Quantity != nil && *parsed.Quantity >= 1 {
		suggestion.Quantity = *parsed.Quantity
	}

	if parsed.Confidence != nil {
		suggestion.Confidence = min(max(*parsed.Confidence, 0), 1)
	}

	return suggestion, nil
}

func truncateAIString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
