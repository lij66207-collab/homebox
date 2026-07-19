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

	// aiBatchMaxTextLen caps the free-form text accepted by ParseBatchText.
	aiBatchMaxTextLen = 4000

	// aiBatchMaxItems caps how many parsed items a single batch returns.
	aiBatchMaxItems = 100

	// aiSearchMaxCandidates caps how many of the group's items are presented
	// to the model as search candidates.
	aiSearchMaxCandidates = 500

	// aiSearchMaxResults caps how many matched items a search returns.
	aiSearchMaxResults = 100

	// aiSearchMaxQueryLen caps the search text accepted by SearchItems.
	aiSearchMaxQueryLen = 200

	// maxAIAncestorDepth bounds the parent-chain walk when resolving an
	// item's location for the search prompt.
	maxAIAncestorDepth = 64
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

// EntityAIBatchItem is one parsed entry from a free-form batch inventory
// text. LocationID is set only when the location named in the text matches an
// existing location in the acting user's group; LocationName echoes what the
// text said (empty when the text named no location).
type EntityAIBatchItem struct {
	Name         string  `json:"name"`
	Quantity     float64 `json:"quantity"`
	LocationID   *string `json:"locationId"`
	LocationName string  `json:"locationName"`
}

// EntityAIBatchResult is the outcome of parsing a batch inventory text.
type EntityAIBatchResult struct {
	Items []EntityAIBatchItem `json:"items"`
}

// EntityAISearchResult is the outcome of an AI semantic item search: the
// matched items, ordered by the model's relevance ranking.
type EntityAISearchResult struct {
	Items []repo.EntitySummary `json:"items"`
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
	content, err := svc.chat(ctx, []aiChatMessage{
		{Role: "system", Content: buildAISystemPrompt(candidates, svc.config.Language)},
		{Role: "user", Content: []aiContentPart{
			{Type: "text", Text: "Analyze this photo of a household item and reply with the JSON object described in the system instructions."},
			{Type: "image_url", ImageURL: &aiImageURL{URL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image)}},
		}},
	})
	if err != nil {
		return EntityAISuggestion{}, err
	}

	return parseAISuggestion(content, candidates)
}

// chat sends a chat-completions request to the configured OpenAI-compatible
// endpoint and returns the first choice's message content.
func (svc *AIService) chat(ctx context.Context, messages []aiChatMessage) (string, error) {
	payload, err := json.Marshal(aiChatRequest{
		Model:    svc.config.Model,
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling ai request: %w", err)
	}

	endpoint := strings.TrimSuffix(svc.config.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("building ai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+svc.config.APIKey)

	resp, err := svc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, aiMaxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading ai response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet := string(body)
		if len(snippet) > aiErrorBodySnippet {
			snippet = snippet[:aiErrorBodySnippet] + "..."
		}
		return "", fmt.Errorf("ai endpoint returned status %d: %s", resp.StatusCode, snippet)
	}

	var chatResp aiChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("decoding ai response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("ai endpoint returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// ParseBatchText parses a free-form inventory list (e.g. "物品A在箱子1，物品B
// 在箱子2") into structured batch items with the configured chat model.
// Locations are matched against the group's existing locations by name;
// unmatched names keep LocationName but get a nil LocationID.
func (svc *AIService) ParseBatchText(ctx context.Context, groupID uuid.UUID, text string) (EntityAIBatchResult, error) {
	spanCtx, span := entityServiceTracer().Start(ctx, "service.AIService.ParseBatchText",
		trace.WithAttributes(
			attribute.String("group.id", groupID.String()),
			attribute.String("ai.model", svc.config.Model),
			attribute.Int("text.length", len(text)),
		))
	defer span.End()
	ctx = spanCtx

	if !svc.config.Enabled || svc.config.APIKey == "" {
		return EntityAIBatchResult{}, ErrAIDisabled
	}

	text = strings.TrimSpace(text)
	if text == "" {
		err := errors.New("batch text is empty")
		recordServiceSpanError(span, err)
		return EntityAIBatchResult{}, err
	}
	if len(text) > aiBatchMaxTextLen {
		err := fmt.Errorf("batch text exceeds %d characters", aiBatchMaxTextLen)
		recordServiceSpanError(span, err)
		return EntityAIBatchResult{}, err
	}

	candidates, err := svc.loadCandidates(ctx, groupID)
	if err != nil {
		recordServiceSpanError(span, err)
		return EntityAIBatchResult{}, err
	}

	content, err := svc.chat(ctx, []aiChatMessage{
		{Role: "system", Content: buildAIBatchPrompt(candidates.Locations, svc.config.Language)},
		{Role: "user", Content: text},
	})
	if err != nil {
		recordServiceSpanError(span, err)
		return EntityAIBatchResult{}, err
	}

	return parseAIBatch(content, candidates.Locations)
}

// SearchItems performs a semantic search over the group's items: the model
// picks the matching items from the candidate list (synonyms, categories,
// loose descriptions — e.g. "刀子" matches 菜刀/水果刀/瑞士军刀), and the
// matches are returned in the model's relevance order. Unknown ids in the
// model reply are dropped.
func (svc *AIService) SearchItems(ctx context.Context, groupID uuid.UUID, query string) (EntityAISearchResult, error) {
	spanCtx, span := entityServiceTracer().Start(ctx, "service.AIService.SearchItems",
		trace.WithAttributes(
			attribute.String("group.id", groupID.String()),
			attribute.String("ai.model", svc.config.Model),
			attribute.Int("query.length", len(query)),
		))
	defer span.End()
	ctx = spanCtx

	if !svc.config.Enabled || svc.config.APIKey == "" {
		return EntityAISearchResult{}, ErrAIDisabled
	}

	query = strings.TrimSpace(query)
	if query == "" {
		err := errors.New("search query is empty")
		recordServiceSpanError(span, err)
		return EntityAISearchResult{}, err
	}
	if len(query) > aiSearchMaxQueryLen {
		err := fmt.Errorf("search query exceeds %d characters", aiSearchMaxQueryLen)
		recordServiceSpanError(span, err)
		return EntityAISearchResult{}, err
	}

	all, err := svc.repos.Entities.GetAll(ctx, groupID)
	if err != nil {
		recordServiceSpanError(span, err)
		return EntityAISearchResult{}, err
	}

	// GetAll does not resolve EntityOut.Location, so walk the parent chains
	// in memory to find each item's nearest location-type ancestor.
	byEntityID := make(map[uuid.UUID]*repo.EntityOut, len(all))
	for i := range all {
		byEntityID[all[i].ID] = &all[i]
	}
	locationOf := func(e *repo.EntityOut) string {
		for p, depth := e.Parent, 0; p != nil && depth < maxAIAncestorDepth; depth++ {
			pe, ok := byEntityID[p.ID]
			if !ok {
				break
			}
			if pe.EntityType != nil && pe.EntityType.IsLocation {
				return pe.Name
			}
			p = pe.Parent
		}
		return "-"
	}

	candidates := make([]aiCandidate, 0, len(all))
	byID := make(map[string]repo.EntitySummary, len(all))
	for i := range all {
		e := &all[i]
		if e.EntityType != nil && e.EntityType.IsLocation {
			continue
		}
		candidates = append(candidates, aiCandidate{ID: e.ID.String(), Name: e.Name, Path: locationOf(e)})
		byID[e.ID.String()] = e.EntitySummary
	}

	if len(candidates) > aiSearchMaxCandidates {
		err := fmt.Errorf("too many items for ai search: %d exceeds %d", len(candidates), aiSearchMaxCandidates)
		recordServiceSpanError(span, err)
		return EntityAISearchResult{}, err
	}
	span.SetAttributes(attribute.Int("search.candidates.count", len(candidates)))

	content, err := svc.chat(ctx, []aiChatMessage{
		{Role: "system", Content: buildAISearchPrompt(candidates)},
		{Role: "user", Content: query},
	})
	if err != nil {
		recordServiceSpanError(span, err)
		return EntityAISearchResult{}, err
	}

	return parseAISearch(content, byID)
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

// buildAIBatchPrompt renders the instruction prompt for parsing a free-form
// batch inventory text. The model must restrict location names to the
// candidate list (matched back to ids server-side).
func buildAIBatchPrompt(locations []aiCandidate, language string) string {
	if language == "" {
		language = "English"
	}

	var sb strings.Builder

	sb.WriteString(`You are parsing a free-form inventory list for a home-inventory app.
The user text names household items and may say which location each item belongs to.
Reply with ONLY a JSON object (no markdown, no prose, no code fences) of this exact shape:
{"items":[{"name": string, "quantity": number, "location": string|null}]}

Rules:
- Split the text into individual items; one array entry per item.
- "name" is a short item name in ` + language + `.
- "quantity" is the count for that item; use 1 when unspecified; never below 1.
- "location" must be copied EXACTLY from the candidate location names below, or null when the text names no location or no candidate fits.
- Do not invent items that are not mentioned in the text.

Candidate locations:
`)
	if len(locations) == 0 {
		sb.WriteString("(none)\n")
	}
	for _, l := range locations {
		fmt.Fprintf(&sb, "- %s\n", l.Name)
	}

	return sb.String()
}

// parseAIBatch decodes the model reply for a batch parse and validates it
// against the candidate locations: names are trimmed, empty entries dropped,
// quantity clamped to >= 1, and location names resolved to ids
// case-insensitively. Unmatched locations keep their echoed name only.
func parseAIBatch(content string, locations []aiCandidate) (EntityAIBatchResult, error) {
	raw := extractJSONObject(content)
	if raw == "" {
		return EntityAIBatchResult{}, fmt.Errorf("no JSON object found in ai reply: %q", truncateAIString(content, aiErrorBodySnippet))
	}

	var parsed struct {
		Items []struct {
			Name     string   `json:"name"`
			Quantity *float64 `json:"quantity"`
			Location *string  `json:"location"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return EntityAIBatchResult{}, fmt.Errorf("decoding ai batch: %w", err)
	}

	lookup := make(map[string]string, len(locations))
	for _, l := range locations {
		key := strings.ToLower(strings.TrimSpace(l.Name))
		if _, ok := lookup[key]; !ok {
			lookup[key] = l.ID
		}
	}

	out := EntityAIBatchResult{Items: []EntityAIBatchItem{}}
	for _, it := range parsed.Items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}

		item := EntityAIBatchItem{Name: name, Quantity: 1}
		if it.Quantity != nil && *it.Quantity >= 1 {
			item.Quantity = *it.Quantity
		}

		if it.Location != nil {
			locName := strings.TrimSpace(*it.Location)
			item.LocationName = locName
			if id, ok := lookup[strings.ToLower(locName)]; ok {
				locID := id
				item.LocationID = &locID
			}
		}

		out.Items = append(out.Items, item)
		if len(out.Items) >= aiBatchMaxItems {
			break
		}
	}

	return out, nil
}

// buildAISearchPrompt renders the instruction prompt for semantic item
// search, listing the group's items (id | name | location) the model must
// restrict its selection to.
func buildAISearchPrompt(candidates []aiCandidate) string {
	var sb strings.Builder

	sb.WriteString(`You are helping search a home-inventory app.
Given the user's search text and the candidate item list, select ALL items that match the text — including synonyms, category matches, and loose descriptions (e.g. "刀子" matches 菜刀/水果刀/瑞士军刀, "electronics" matches a phone or a laptop).
Reply with ONLY a JSON object (no markdown, no prose, no code fences) of this exact shape:
{"itemIds": [string]}

Rules:
- Only use ids from the candidate list below; never invent ids.
- Order the ids by relevance, best match first.
- If nothing matches, reply {"itemIds": []}.

Candidate items (id | name | location):
`)
	if len(candidates) == 0 {
		sb.WriteString("(none)\n")
	}
	for _, c := range candidates {
		fmt.Fprintf(&sb, "- %s | %s | %s\n", c.ID, c.Name, c.Path)
	}

	return sb.String()
}

// parseAISearch decodes the model reply for an item search: ids are
// validated against the candidate set (unknown/duplicate ids dropped) and
// mapped back to their entity summaries, preserving the model's relevance
// order.
func parseAISearch(content string, byID map[string]repo.EntitySummary) (EntityAISearchResult, error) {
	raw := extractJSONObject(content)
	if raw == "" {
		return EntityAISearchResult{}, fmt.Errorf("no JSON object found in ai reply: %q", truncateAIString(content, aiErrorBodySnippet))
	}

	var parsed struct {
		ItemIDs []string `json:"itemIds"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return EntityAISearchResult{}, fmt.Errorf("decoding ai search: %w", err)
	}

	out := EntityAISearchResult{Items: []repo.EntitySummary{}}
	seen := make(map[string]struct{}, len(parsed.ItemIDs))
	for _, id := range parsed.ItemIDs {
		id = strings.TrimSpace(id)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		if item, ok := byID[id]; ok {
			out.Items = append(out.Items, item)
			if len(out.Items) >= aiSearchMaxResults {
				break
			}
		}
	}

	return out, nil
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
