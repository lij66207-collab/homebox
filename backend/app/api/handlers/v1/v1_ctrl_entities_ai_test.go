package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services/reporting/eventbus"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	_ "github.com/sysadminsmedia/homebox/backend/pkgs/cgofreesqlite"
)

// aiTestAPIKey is the fake API key the AI handler tests configure and assert on.
const aiTestAPIKey = "test-key"

func TestHandleEntityAISuggest_ServiceUnavailableWhenDisabled(t *testing.T) {
	ctrl := NewControllerV1(nil, nil, nil, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-suggest", nil)
	rec := httptest.NewRecorder()

	err := ctrl.HandleEntityAISuggest()(rec, req)
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusServiceUnavailable, reqErr.Status)
}

func TestHandleEntityAISuggest_HappyPath(t *testing.T) {
	ctx := context.Background()

	// Mock OpenAI-compatible endpoint
	aiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"name\":\"Wrench\",\"description\":\"A steel wrench.\",\"quantity\":2,\"suggestedTagIds\":[],\"suggestedLocationId\":null,\"confidence\":0.8}"}}]}`))
	}))
	t.Cleanup(aiSrv.Close)

	// In-memory database + real repos/services
	client, err := ent.Open("sqlite3", "file:aihandler?mode=memory&cache=shared&_fk=1&_time_format=sqlite")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Schema.Create(ctx))

	bus := eventbus.New()
	go func() { _ = bus.Run(ctx) }()

	repos := repo.New(client, bus, config.Storage{
		PrefixPath: "/",
		ConnString: "file://" + os.TempDir(),
	}, "mem://{{ .Topic }}", config.Thumbnail{})

	group, err := repos.Groups.GroupCreate(ctx, "ai-handler-test-group", uuid.Nil)
	require.NoError(t, err)

	aiConf := config.AIConf{
		Enabled: true,
		APIKey:  aiTestAPIKey,
		BaseURL: aiSrv.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}

	svc := services.New(repos, services.WithAIConfig(&aiConf))
	ctrl := NewControllerV1(svc, repos, bus, &config.Config{AI: aiConf}, WithMaxUploadSize(10))

	// Multipart request with the photo under the "file" field
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="photo.png"`)
	header.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-png-bytes"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-suggest", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(services.SetTenantCtx(req.Context(), group.ID))
	rec := httptest.NewRecorder()

	err = ctrl.HandleEntityAISuggest()(rec, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var suggestion services.EntityAISuggestion
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &suggestion))
	assert.Equal(t, "Wrench", suggestion.Name)
	assert.Equal(t, "A steel wrench.", suggestion.Description)
	assert.InDelta(t, 2.0, suggestion.Quantity, 0.0001)
	assert.Empty(t, suggestion.SuggestedTagIDs)
	assert.Nil(t, suggestion.SuggestedLocationID)
	assert.InDelta(t, 0.8, suggestion.Confidence, 0.0001)
}

func TestHandleEntityAISuggest_RejectsNonImage(t *testing.T) {
	ctrl := NewControllerV1(nil, nil, nil, &config.Config{
		AI: config.AIConf{Enabled: true, APIKey: aiTestAPIKey},
	})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="notes.txt"`)
	header.Set("Content-Type", "text/plain")
	part, err := mw.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write([]byte("not an image"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-suggest", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	err = ctrl.HandleEntityAISuggest()(rec, req)
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusUnprocessableEntity, reqErr.Status)
}

func TestHandleEntityAIBatchParse_ServiceUnavailableWhenDisabled(t *testing.T) {
	ctrl := NewControllerV1(nil, nil, nil, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-batch-parse", nil)
	rec := httptest.NewRecorder()

	err := ctrl.HandleEntityAIBatchParse()(rec, req)
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusServiceUnavailable, reqErr.Status)
}

func TestHandleEntityAIBatchParse_RejectsEmptyText(t *testing.T) {
	ctrl := NewControllerV1(nil, nil, nil, &config.Config{
		AI: config.AIConf{Enabled: true, APIKey: aiTestAPIKey},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-batch-parse",
		bytes.NewBufferString(`{"text":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	err := ctrl.HandleEntityAIBatchParse()(rec, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHandleEntityAIBatchParse_HappyPath(t *testing.T) {
	ctx := context.Background()

	aiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"items\":[{\"name\":\"螺丝刀\",\"quantity\":2,\"location\":\"储藏间\"},{\"name\":\"锤子\",\"quantity\":1,\"location\":null}]}"}}]}`))
	}))
	t.Cleanup(aiSrv.Close)

	client, err := ent.Open("sqlite3", "file:aibatchhandler?mode=memory&cache=shared&_fk=1&_time_format=sqlite")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Schema.Create(ctx))

	bus := eventbus.New()
	go func() { _ = bus.Run(ctx) }()

	repos := repo.New(client, bus, config.Storage{
		PrefixPath: "/",
		ConnString: "file://" + os.TempDir(),
	}, "mem://{{ .Topic }}", config.Thumbnail{})

	group, err := repos.Groups.GroupCreate(ctx, "ai-batch-handler-test-group", uuid.Nil)
	require.NoError(t, err)

	loc, err := repos.Entities.CreateContainer(ctx, group.ID, repo.EntityCreate{Name: "储藏间"})
	require.NoError(t, err)

	aiConf := config.AIConf{
		Enabled: true,
		APIKey:  aiTestAPIKey,
		BaseURL: aiSrv.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}

	svc := services.New(repos, services.WithAIConfig(&aiConf))
	ctrl := NewControllerV1(svc, repos, bus, &config.Config{AI: aiConf}, WithMaxUploadSize(10))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-batch-parse",
		bytes.NewBufferString(`{"text":"螺丝刀两把在储藏间，锤子一把"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(services.SetTenantCtx(req.Context(), group.ID))
	rec := httptest.NewRecorder()

	err = ctrl.HandleEntityAIBatchParse()(rec, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var result services.EntityAIBatchResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Len(t, result.Items, 2)

	assert.Equal(t, "螺丝刀", result.Items[0].Name)
	assert.InDelta(t, 2.0, result.Items[0].Quantity, 0.0001)
	require.NotNil(t, result.Items[0].LocationID)
	assert.Equal(t, loc.ID.String(), *result.Items[0].LocationID)

	assert.Equal(t, "锤子", result.Items[1].Name)
	assert.Nil(t, result.Items[1].LocationID)
}

func TestHandleEntityAISearch_ServiceUnavailableWhenDisabled(t *testing.T) {
	ctrl := NewControllerV1(nil, nil, nil, &config.Config{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-search", nil)
	rec := httptest.NewRecorder()

	err := ctrl.HandleEntityAISearch()(rec, req)
	require.Error(t, err)

	var reqErr *validate.RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusServiceUnavailable, reqErr.Status)
}

func TestHandleEntityAISearch_HappyPath(t *testing.T) {
	ctx := context.Background()

	client, err := ent.Open("sqlite3", "file:aisearchhandler?mode=memory&cache=shared&_fk=1&_time_format=sqlite")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Schema.Create(ctx))

	bus := eventbus.New()
	go func() { _ = bus.Run(ctx) }()

	repos := repo.New(client, bus, config.Storage{
		PrefixPath: "/",
		ConnString: "file://" + os.TempDir(),
	}, "mem://{{ .Topic }}", config.Thumbnail{})

	group, err := repos.Groups.GroupCreate(ctx, "ai-search-handler-test-group", uuid.Nil)
	require.NoError(t, err)

	itemType, err := repos.EntityTypes.GetDefault(ctx, group.ID, false)
	require.NoError(t, err)

	item, err := repos.Entities.Create(ctx, group.ID, repo.EntityCreate{Name: "菜刀", EntityTypeID: itemType.ID, AssetID: 123})
	require.NoError(t, err)

	aiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":"{\"itemIds\":[\"%s\"]}"}}]}`, item.ID.String())
	}))
	t.Cleanup(aiSrv.Close)

	aiConf := config.AIConf{
		Enabled: true,
		APIKey:  aiTestAPIKey,
		BaseURL: aiSrv.URL,
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}

	svc := services.New(repos, services.WithAIConfig(&aiConf))
	ctrl := NewControllerV1(svc, repos, bus, &config.Config{AI: aiConf}, WithMaxUploadSize(10))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/entities/ai-search",
		bytes.NewBufferString(`{"query":"刀子"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(services.SetTenantCtx(req.Context(), group.ID))
	rec := httptest.NewRecorder()

	err = ctrl.HandleEntityAISearch()(rec, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var result services.EntityAISearchResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Len(t, result.Items, 1)
	assert.Equal(t, item.ID, result.Items[0].ID)
	assert.Equal(t, "菜刀", result.Items[0].Name)
}
