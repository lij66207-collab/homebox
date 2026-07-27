package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

// sttTestModel is the STT model name used across the assistant tests.
const sttTestModel = "whisper-1"

// capturedSTTRequest records what the mock transcription endpoint received so
// tests can assert on the outgoing multipart request shape.
type capturedSTTRequest struct {
	path       string
	authHeader string
	model      string
	filename   string
	fileBytes  []byte
}

// newMockSTTServer returns an httptest server that captures the incoming
// transcription request and replies with status/reply.
func newMockSTTServer(t *testing.T, status int, reply string, capture *capturedSTTRequest) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.path = r.URL.Path
		capture.authHeader = r.Header.Get("Authorization")

		if err := r.ParseMultipartForm(AssistantMaxAudioBytes); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		capture.model = r.FormValue("model")
		if files := r.MultipartForm.File["file"]; len(files) > 0 {
			capture.filename = files[0].Filename
			f, err := files[0].Open()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer func() { _ = f.Close() }()
			capture.fileBytes, _ = io.ReadAll(f)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, reply)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestAIService_TranscribeAudio_HappyPath(t *testing.T) {
	capture := &capturedSTTRequest{}
	srv := newMockSTTServer(t, http.StatusOK, `{"text":"风扇在002位置"}`, capture)

	svc := newTestAIService("http://unused.invalid", 5*time.Second)
	audio := []byte("fake-audio-bytes")

	text, err := svc.TranscribeAudio(context.Background(), audio, "audio/webm;codecs=opus", STTConfig{
		BaseURL: srv.URL + "/v1",
		APIKey:  "stt-secret",
		Model:   sttTestModel,
	})
	require.NoError(t, err)
	assert.Equal(t, "风扇在002位置", text)

	assert.Equal(t, "/v1/audio/transcriptions", capture.path)
	assert.Equal(t, "Bearer stt-secret", capture.authHeader)
	assert.Equal(t, sttTestModel, capture.model)
	assert.Equal(t, "audio.webm", capture.filename)
	assert.Equal(t, audio, capture.fileBytes)
}

func TestAIService_TranscribeAudio_Non200(t *testing.T) {
	capture := &capturedSTTRequest{}
	srv := newMockSTTServer(t, http.StatusInternalServerError, `{"error":"whisper exploded"}`, capture)

	svc := newTestAIService("http://unused.invalid", 5*time.Second)

	_, err := svc.TranscribeAudio(context.Background(), []byte("audio"), "audio/webm", STTConfig{
		BaseURL: srv.URL,
		APIKey:  "stt-secret",
		Model:   sttTestModel,
	})
	require.ErrorContains(t, err, "500")
	assert.Contains(t, err.Error(), "whisper exploded")
}

func TestAIService_TranscribeAudio_EmptyAudio(t *testing.T) {
	svc := newTestAIService("http://unused.invalid", 5*time.Second)
	_, err := svc.TranscribeAudio(context.Background(), nil, "audio/webm", STTConfig{
		BaseURL: "http://unused.invalid",
		APIKey:  "stt-secret",
		Model:   sttTestModel,
	})
	require.ErrorContains(t, err, "empty")
}

func TestAIService_TranscribeAudio_TooLarge(t *testing.T) {
	svc := newTestAIService("http://unused.invalid", 5*time.Second)
	_, err := svc.TranscribeAudio(context.Background(), make([]byte, AssistantMaxAudioBytes+1), "audio/webm", STTConfig{
		BaseURL: "http://unused.invalid",
		APIKey:  "stt-secret",
		Model:   sttTestModel,
	})
	require.ErrorContains(t, err, "exceeds")
}

func TestAIService_TranscribeAudio_IncompleteConfig(t *testing.T) {
	svc := newTestAIService("http://unused.invalid", 5*time.Second)
	for _, cfg := range []STTConfig{
		{},
		{BaseURL: "http://x", APIKey: "k"},
		{BaseURL: "http://x", Model: "m"},
		{APIKey: "k", Model: "m"},
	} {
		_, err := svc.TranscribeAudio(context.Background(), []byte("audio"), "audio/webm", cfg)
		assert.ErrorIs(t, err, ErrAssistantSTTNotConfigured, "config %+v", cfg)
	}
}

func TestAssistantAudioExtension(t *testing.T) {
	cases := []struct {
		mime string
		want string
	}{
		{"audio/webm", "webm"},
		{"audio/webm;codecs=opus", "webm"},
		{"audio/ogg", "ogg"},
		{"audio/mpeg", "mp3"},
		{"audio/wav", "wav"},
		{"audio/x-m4a", "m4a"},
		{"audio/mp4", "m4a"},
		{"application/octet-stream", "webm"},
		{"", "webm"},
	}

	for _, tc := range cases {
		t.Run(tc.mime, func(t *testing.T) {
			assert.Equal(t, tc.want, assistantAudioExtension(tc.mime))
		})
	}
}

// capturedAssistantChat records the full message list of a chat-completions
// request (all assistant messages carry plain string content).
type capturedAssistantChat struct {
	path     string
	model    string
	messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
}

// newMockAssistantChatServer returns an httptest server that captures the
// chat-completions request and replies with the given body.
func newMockAssistantChatServer(t *testing.T, status int, reply string, capture *capturedAssistantChat) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.path = r.URL.Path

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var decoded struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		capture.model = decoded.Model
		capture.messages = decoded.Messages

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, reply)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestAIService_ParseAssistantCommand_HappyPath(t *testing.T) {
	ctx := context.Background()

	loc, err := tRepos.Entities.CreateContainer(ctx, tGroup.ID, repo.EntityCreate{Name: "assistant-loc-" + fk.Str(6)})
	require.NoError(t, err)

	reply := aiReplyJSON(t, `{"reply":"我找到了 002 位置，要在这里新建风扇物品吗？","actions":[`+
		`{"type":"create_location","name":"004","parent_path":null},`+
		`{"type":"create_item","name":"风扇","location_path":"002","quantity":1,"description":null},`+
		`{"type":"query_item","keyword":"螺丝刀"},`+
		`{"type":"query_location","keyword":"002"},`+
		`{"type":"delete_item","name":"不该出现的动作"},`+
		`{"type":"create_item","name":"","location_path":null},`+
		`{"type":"create_item","name":"钉子","quantity":0}`+
		`]}`)

	capture := &capturedAssistantChat{}
	srv := newMockAssistantChatServer(t, http.StatusOK, reply, capture)
	svc := newTestAIService(srv.URL+"/v1", 5*time.Second)

	history := []AssistantMessage{
		{Role: aiChatRoleSystem, Content: "must be dropped"},
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好，有什么可以帮你？"},
		{Role: "user", Content: "   "},
	}

	result, err := svc.ParseAssistantCommand(ctx, tGroup.ID, history, "帮我在002位置新建一个风扇")
	require.NoError(t, err)

	assert.Equal(t, "我找到了 002 位置，要在这里新建风扇物品吗？", result.Reply)
	require.Len(t, result.Actions, 5, "unknown type and empty-name actions should be dropped")

	assert.Equal(t, AssistantActionCreateLocation, result.Actions[0].Type)
	assert.Equal(t, "004", result.Actions[0].Name)
	assert.Nil(t, result.Actions[0].ParentPath)

	assert.Equal(t, AssistantActionCreateItem, result.Actions[1].Type)
	assert.Equal(t, "风扇", result.Actions[1].Name)
	require.NotNil(t, result.Actions[1].LocationPath)
	assert.Equal(t, "002", *result.Actions[1].LocationPath)
	require.NotNil(t, result.Actions[1].Quantity)
	assert.InDelta(t, 1.0, *result.Actions[1].Quantity, 0.0001)

	assert.Equal(t, AssistantActionQueryItem, result.Actions[2].Type)
	assert.Equal(t, "螺丝刀", result.Actions[2].Keyword)
	assert.Equal(t, AssistantActionQueryLocation, result.Actions[3].Type)
	assert.Equal(t, "002", result.Actions[3].Keyword)

	assert.Equal(t, "钉子", result.Actions[4].Name)
	assert.Nil(t, result.Actions[4].Quantity, "quantity below 1 should be nulled")

	// Request shape: system + 2 valid history turns + current user text.
	require.Len(t, capture.messages, 4)
	assert.Equal(t, "system", capture.messages[0].Role)
	assert.Contains(t, capture.messages[0].Content, loc.Name, "system prompt should list candidate location paths")
	assert.Equal(t, "user", capture.messages[1].Role)
	assert.Equal(t, "你好", capture.messages[1].Content)
	assert.Equal(t, "assistant", capture.messages[2].Role)
	assert.Equal(t, "user", capture.messages[3].Role)
	assert.Equal(t, "帮我在002位置新建一个风扇", capture.messages[3].Content)
}

func TestAIService_ParseAssistantCommand_HistoryTruncated(t *testing.T) {
	reply := aiReplyJSON(t, `{"reply":"好的","actions":[]}`)

	capture := &capturedAssistantChat{}
	srv := newMockAssistantChatServer(t, http.StatusOK, reply, capture)
	svc := newTestAIService(srv.URL, 5*time.Second)

	history := make([]AssistantMessage, 0, 14)
	for i := range 14 {
		history = append(history, AssistantMessage{Role: "user", Content: fmt.Sprintf("消息%02d", i)})
	}

	_, err := svc.ParseAssistantCommand(context.Background(), tGroup.ID, history, "最新问题")
	require.NoError(t, err)

	// system + last 10 history turns + current text
	require.Len(t, capture.messages, 12)
	assert.Equal(t, "消息04", capture.messages[1].Content, "oldest history turns should be dropped")
	assert.Equal(t, "消息13", capture.messages[10].Content)
	assert.Equal(t, "最新问题", capture.messages[11].Content)
}

func TestAIService_ParseAssistantCommand_DegradesOnBadJSON(t *testing.T) {
	capture := &capturedAssistantChat{}
	srv := newMockAssistantChatServer(t, http.StatusOK, aiReplyJSON(t, "抱歉，我没听懂，请再说一遍。"), capture)
	svc := newTestAIService(srv.URL, 5*time.Second)

	result, err := svc.ParseAssistantCommand(context.Background(), tGroup.ID, nil, "嗯……")
	require.NoError(t, err)
	assert.Equal(t, "抱歉，我没听懂，请再说一遍。", result.Reply)
	assert.Empty(t, result.Actions)
}

func TestAIService_ParseAssistantCommand_DegradesOnBrokenProtocol(t *testing.T) {
	// Valid JSON, but not the protocol shape.
	capture := &capturedAssistantChat{}
	srv := newMockAssistantChatServer(t, http.StatusOK, aiReplyJSON(t, `{"reply": 42, "actions": "nope"}`), capture)
	svc := newTestAIService(srv.URL, 5*time.Second)

	result, err := svc.ParseAssistantCommand(context.Background(), tGroup.ID, nil, "随便说点什么")
	require.NoError(t, err)
	assert.Contains(t, result.Reply, `"reply": 42`, "unparseable protocol JSON should degrade to the raw reply text")
	assert.Empty(t, result.Actions)
}

func TestAIService_ParseAssistantCommand_EmptyText(t *testing.T) {
	svc := newTestAIService("http://unused.invalid", 5*time.Second)
	_, err := svc.ParseAssistantCommand(context.Background(), tGroup.ID, nil, "  ")
	require.ErrorContains(t, err, "empty")
}

func TestAIService_ParseAssistantCommand_Disabled(t *testing.T) {
	svc := NewAIService(tRepos, config.AIConf{})
	_, err := svc.ParseAssistantCommand(context.Background(), tGroup.ID, nil, "新建位置")
	assert.ErrorIs(t, err, ErrAIDisabled)
}

func TestAssistantSTTConfig(t *testing.T) {
	cfg, err := AssistantSTTConfig(map[string]interface{}{
		settingsNamespaceAssistant: map[string]interface{}{
			settingsKeyAssistantEnabled: true,
			settingsKeySTTBaseURL:       "https://stt.example.com/v1",
			settingsKeySTTAPIKey:        "secret",
			settingsKeySTTModel:         sttTestModel,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, STTConfig{BaseURL: "https://stt.example.com/v1", APIKey: "secret", Model: sttTestModel}, cfg)

	_, err = AssistantSTTConfig(map[string]interface{}{})
	require.ErrorIs(t, err, ErrAssistantDisabled)

	_, err = AssistantSTTConfig(map[string]interface{}{
		settingsNamespaceAssistant: map[string]interface{}{
			settingsKeyAssistantEnabled: false,
		},
	})
	require.ErrorIs(t, err, ErrAssistantDisabled)

	_, err = AssistantSTTConfig(map[string]interface{}{
		settingsNamespaceAssistant: map[string]interface{}{
			settingsKeyAssistantEnabled: true,
			settingsKeySTTBaseURL:       "https://stt.example.com/v1",
		},
	})
	require.ErrorIs(t, err, ErrAssistantSTTNotConfigured)
}
