package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

// capturedAIRequest records what the mock AI endpoint received so tests can
// assert on the outgoing request shape.
type capturedAIRequest struct {
	path       string
	authHeader string
	model      string
	system     string
	userParts  []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
}

// newMockAIServer returns an httptest server that captures the incoming
// chat-completions request and replies with status/reply.
func newMockAIServer(t *testing.T, status int, reply string, capture *capturedAIRequest) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.path = r.URL.Path
		capture.authHeader = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var decoded struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		capture.model = decoded.Model
		for _, msg := range decoded.Messages {
			switch msg.Role {
			case "system":
				_ = json.Unmarshal(msg.Content, &capture.system)
			case "user":
				_ = json.Unmarshal(msg.Content, &capture.userParts)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, reply)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// aiReplyJSON wraps raw content in an OpenAI-compatible chat completion body.
func aiReplyJSON(t *testing.T, content string) string {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
	})
	require.NoError(t, err)
	return string(b)
}

func newTestAIService(baseURL string, timeout time.Duration) *AIService {
	return NewAIService(tRepos, config.AIConf{
		Enabled: true,
		APIKey:  "test-api-key",
		BaseURL: baseURL,
		Model:   "test-model",
		Timeout: timeout,
	})
}

// setupAICandidates creates one tag and one location in the shared test group
// and returns their IDs.
func setupAICandidates(t *testing.T) (tagID string, locID string) {
	t.Helper()

	ctx := context.Background()

	tag, err := tRepos.Tags.Create(ctx, tGroup.ID, repo.TagCreate{Name: "ai-tag-" + fk.Str(8)})
	require.NoError(t, err)

	loc, err := tRepos.Entities.CreateContainer(ctx, tGroup.ID, repo.EntityCreate{Name: "ai-loc-" + fk.Str(8)})
	require.NoError(t, err)

	return tag.ID.String(), loc.ID.String()
}

func TestAIService_SuggestFromPhoto_HappyPath(t *testing.T) {
	tagID, locID := setupAICandidates(t)

	reply := aiReplyJSON(t, fmt.Sprintf(
		`{"name":"Wrench","description":"A steel combination wrench.","quantity":2,"suggestedTagIds":[%q,"does-not-exist"],"suggestedLocationId":%q,"confidence":0.9}`,
		tagID, locID))

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)

	svc := newTestAIService(srv.URL+"/v1", 5*time.Second)
	image := []byte("fake-image-bytes")

	suggestion, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, image, "image/png")
	require.NoError(t, err)

	// Request shape
	assert.Equal(t, "/v1/chat/completions", capture.path)
	assert.Equal(t, "Bearer test-api-key", capture.authHeader)
	assert.Equal(t, "test-model", capture.model)
	assert.Contains(t, capture.system, tagID, "system prompt should contain candidate tag ids")
	assert.Contains(t, capture.system, locID, "system prompt should contain candidate location ids")
	assert.Contains(t, capture.system, "path:", "system prompt should contain location paths")

	var foundImage bool
	for _, part := range capture.userParts {
		if part.Type == "image_url" {
			foundImage = true
			assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(image), part.ImageURL.URL)
		}
	}
	assert.True(t, foundImage, "expected an image_url content part in the user message")

	// Response mapping (unknown tag id filtered out)
	assert.Equal(t, "Wrench", suggestion.Name)
	assert.Equal(t, "A steel combination wrench.", suggestion.Description)
	assert.InDelta(t, 2.0, suggestion.Quantity, 0.0001)
	assert.Equal(t, []string{tagID}, suggestion.SuggestedTagIDs)
	require.NotNil(t, suggestion.SuggestedLocationID)
	assert.Equal(t, locID, *suggestion.SuggestedLocationID)
	assert.InDelta(t, 0.9, suggestion.Confidence, 0.0001)
}

func TestAIService_SuggestFromPhoto_FencedReply(t *testing.T) {
	tagID, _ := setupAICandidates(t)

	fenced := "```json\n" + fmt.Sprintf(
		`{"name":"Hammer","description":"A claw hammer.","quantity":1,"suggestedTagIds":[%q],"suggestedLocationId":null,"confidence":0.5}`,
		tagID) + "\n```"
	reply := aiReplyJSON(t, fenced)

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)

	svc := newTestAIService(srv.URL, 5*time.Second)

	suggestion, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.NoError(t, err)
	assert.Equal(t, "Hammer", suggestion.Name)
	assert.Equal(t, []string{tagID}, suggestion.SuggestedTagIDs)
	assert.Nil(t, suggestion.SuggestedLocationID)
}

func TestAIService_SuggestFromPhoto_SurroundingProse(t *testing.T) {
	reply := aiReplyJSON(t, `Sure! Here is the result: {"name":"Drill","description":"Cordless drill.","quantity":1,"suggestedTagIds":[],"suggestedLocationId":null,"confidence":0.4} Hope that helps!`)

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)

	svc := newTestAIService(srv.URL, 5*time.Second)

	suggestion, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.NoError(t, err)
	assert.Equal(t, "Drill", suggestion.Name)
}

func TestAIService_SuggestFromPhoto_UnknownIDsFiltered(t *testing.T) {
	setupAICandidates(t) // ensure the group has some candidates, none matching the reply

	reply := aiReplyJSON(t, `{"name":"Box","description":"A cardboard box.","quantity":1,"suggestedTagIds":["unknown-tag-1","unknown-tag-2"],"suggestedLocationId":"unknown-location","confidence":0.3}`)

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)

	svc := newTestAIService(srv.URL, 5*time.Second)

	suggestion, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.NoError(t, err)
	assert.Empty(t, suggestion.SuggestedTagIDs)
	assert.Nil(t, suggestion.SuggestedLocationID)
}

func TestAIService_SuggestFromPhoto_ClampsValues(t *testing.T) {
	reply := aiReplyJSON(t, `{"name":"Nails","description":"A box of nails.","quantity":0,"suggestedTagIds":[],"suggestedLocationId":null,"confidence":1.7}`)

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)

	svc := newTestAIService(srv.URL, 5*time.Second)

	suggestion, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, suggestion.Quantity, 0.0001, "quantity below 1 should be clamped to 1")
	assert.InDelta(t, 1.0, suggestion.Confidence, 0.0001, "confidence above 1 should be clamped to 1")
}

func TestAIService_SuggestFromPhoto_MalformedReply(t *testing.T) {
	reply := aiReplyJSON(t, "this is not json at all")

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)

	svc := newTestAIService(srv.URL, 5*time.Second)

	_, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.ErrorContains(t, err, "no JSON object")
}

func TestAIService_SuggestFromPhoto_Non200(t *testing.T) {
	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusInternalServerError, `{"error":"model exploded"}`, capture)

	svc := newTestAIService(srv.URL, 5*time.Second)

	_, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.ErrorContains(t, err, "500")
	assert.Contains(t, err.Error(), "model exploded")
}

func TestAIService_SuggestFromPhoto_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	svc := newTestAIService(srv.URL, 50*time.Millisecond)

	_, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
	require.ErrorContains(t, err, "ai request failed")
}

func TestAIService_SuggestFromPhoto_Disabled(t *testing.T) {
	for _, cfg := range []config.AIConf{
		{},                                   // disabled, no key
		{Enabled: true},                      // enabled but no key
		{Enabled: false, APIKey: "some-key"}, // key but disabled
	} {
		svc := NewAIService(tRepos, cfg)
		_, err := svc.SuggestFromPhoto(context.Background(), tGroup.ID, []byte("img"), "image/jpeg")
		assert.ErrorIs(t, err, ErrAIDisabled, "config %+v", cfg)
	}
}

func TestExtractJSONObject(t *testing.T) {
	const simpleJSON = `{"a":1}`
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: simpleJSON, want: simpleJSON},
		{name: "whitespace", input: "  \n " + simpleJSON + " \n", want: simpleJSON},
		{name: "json fence", input: "```json\n" + simpleJSON + "\n```", want: simpleJSON},
		{name: "plain fence", input: "```\n" + simpleJSON + "\n```", want: simpleJSON},
		{name: "prose around", input: `here you go {"a":{"b":2}} done`, want: `{"a":{"b":2}}`},
		{name: "no object", input: "no json here", want: ""},
		{name: "unterminated", input: `{"a":1`, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, extractJSONObject(tc.input))
		})
	}
}

func TestAIService_ParseBatchText_HappyPath(t *testing.T) {
	ctx := context.Background()

	loc, err := tRepos.Entities.CreateContainer(ctx, tGroup.ID, repo.EntityCreate{Name: "batch-loc-" + fk.Str(6)})
	require.NoError(t, err)

	reply := aiReplyJSON(t, fmt.Sprintf(
		`{"items":[`+
			`{"name":"螺丝刀","quantity":2,"location":%q},`+
			`{"name":"卷尺","quantity":1,"location":%q},`+
			`{"name":"锤子","quantity":null,"location":null},`+
			`{"name":"扳手","quantity":1,"location":"不存在的箱子"},`+
			`{"name":"  ","quantity":1,"location":null}`+
			`]}`,
		loc.Name, strings.ToUpper(loc.Name)))

	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, reply, capture)
	svc := newTestAIService(srv.URL+"/v1", 5*time.Second)

	result, err := svc.ParseBatchText(ctx, tGroup.ID, "螺丝刀两个、卷尺一个都在"+loc.Name+"，锤子一把，扳手在不存在的箱子")
	require.NoError(t, err)
	require.Len(t, result.Items, 4, "blank-name entry should be dropped")

	assert.Equal(t, "螺丝刀", result.Items[0].Name)
	assert.InDelta(t, 2.0, result.Items[0].Quantity, 0.0001)
	require.NotNil(t, result.Items[0].LocationID)
	assert.Equal(t, loc.ID.String(), *result.Items[0].LocationID)
	assert.Equal(t, loc.Name, result.Items[0].LocationName)

	assert.Equal(t, "卷尺", result.Items[1].Name)
	require.NotNil(t, result.Items[1].LocationID, "location match must be case-insensitive")
	assert.Equal(t, loc.ID.String(), *result.Items[1].LocationID)

	assert.Equal(t, "锤子", result.Items[2].Name)
	assert.InDelta(t, 1.0, result.Items[2].Quantity, 0.0001, "null quantity defaults to 1")
	assert.Nil(t, result.Items[2].LocationID)

	assert.Equal(t, "扳手", result.Items[3].Name)
	assert.Nil(t, result.Items[3].LocationID, "unknown location must not resolve")
	assert.Equal(t, "不存在的箱子", result.Items[3].LocationName)

	assert.Contains(t, capture.system, loc.Name, "system prompt should list candidate location names")
	assert.Equal(t, "Bearer test-api-key", capture.authHeader)
}

func TestAIService_ParseBatchText_EmptyText(t *testing.T) {
	svc := newTestAIService("http://unused.invalid", 5*time.Second)
	_, err := svc.ParseBatchText(context.Background(), tGroup.ID, "   ")
	require.ErrorContains(t, err, "empty")
}

func TestAIService_ParseBatchText_MalformedReply(t *testing.T) {
	capture := &capturedAIRequest{}
	srv := newMockAIServer(t, http.StatusOK, aiReplyJSON(t, "no json here"), capture)
	svc := newTestAIService(srv.URL, 5*time.Second)

	_, err := svc.ParseBatchText(context.Background(), tGroup.ID, "物品A在箱子1")
	require.ErrorContains(t, err, "no JSON object")
}

func TestAIService_ParseBatchText_Disabled(t *testing.T) {
	svc := NewAIService(tRepos, config.AIConf{})
	_, err := svc.ParseBatchText(context.Background(), tGroup.ID, "物品A在箱子1")
	assert.ErrorIs(t, err, ErrAIDisabled)
}
