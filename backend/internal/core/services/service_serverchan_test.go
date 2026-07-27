package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

// serverChanMock is an httptest stand-in for the Server酱 API that records
// the pushes it receives.
type serverChanMock struct {
	*httptest.Server

	mu     sync.Mutex
	pushes []serverChanPush
	code   int
}

type serverChanPush struct {
	path  string
	title string
	desp  string
}

func newServerChanMock(t *testing.T, code int) *serverChanMock {
	t.Helper()
	m := &serverChanMock{code: code}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		m.mu.Lock()
		m.pushes = append(m.pushes, serverChanPush{
			path:  r.URL.EscapedPath(),
			title: r.Form.Get("title"),
			desp:  r.Form.Get("desp"),
		})
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": m.code, "message": "mock message"})
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *serverChanMock) recorded() []serverChanPush {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]serverChanPush(nil), m.pushes...)
}

// withServerChanBaseURL points the Server酱 client at the mock server for the
// duration of the test.
func withServerChanBaseURL(t *testing.T, url string) {
	t.Helper()
	prev := serverChanBaseURL
	serverChanBaseURL = url
	t.Cleanup(func() { serverChanBaseURL = prev })
}

func TestSendServerChan(t *testing.T) {
	mock := newServerChanMock(t, 0)
	withServerChanBaseURL(t, mock.URL)

	err := SendServerChan(context.Background(), "TEST/KEY", "标题", "内容")
	require.NoError(t, err)

	pushes := mock.recorded()
	require.Len(t, pushes, 1)
	// The sendkey must be path-escaped when building the endpoint.
	assert.Equal(t, "/TEST%2FKEY.send", pushes[0].path)
	assert.Equal(t, "标题", pushes[0].title)
	assert.Equal(t, "内容", pushes[0].desp)
}

func TestSendServerChan_APIError(t *testing.T) {
	mock := newServerChanMock(t, 40001)
	withServerChanBaseURL(t, mock.URL)

	err := SendServerChan(context.Background(), "BADKEY", "t", "d")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "40001")
	assert.Contains(t, err.Error(), "mock message")
}

func TestSendServerChan_EmptyKey(t *testing.T) {
	require.Error(t, SendServerChan(context.Background(), "", "t", "d"))
}

func TestParseExpiryReminderConfig_Defaults(t *testing.T) {
	cfg := parseExpiryReminderConfig(map[string]interface{}{})
	assert.False(t, cfg.enabled)
	assert.Empty(t, cfg.sendKey)
	assert.Equal(t, []int{30, 7, 1}, cfg.daysBefore)
	assert.Equal(t, defaultExpiryNotifyHour, cfg.notifyHour)
}

// setExpiryReminderSettings replaces the test group's settings through the
// repo so the document round-trips through JSON (numbers come back as
// float64, exactly as in production).
func setExpiryReminderSettings(t *testing.T, ns map[string]interface{}) {
	t.Helper()
	err := tRepos.Groups.UpdateSettings(context.Background(), tGroup.ID, map[string]interface{}{
		settingsNamespaceExpiryReminder: ns,
	})
	require.NoError(t, err)
}

func createExpiringEntity(t *testing.T, name string, inDays int) {
	t.Helper()
	ctx := context.Background()
	e, err := tRepos.Entities.Create(ctx, tGroup.ID, repo.EntityCreate{Name: name, Quantity: 1})
	require.NoError(t, err)
	_, err = tRepos.Entities.UpdateByGroup(ctx, tGroup.ID, repo.EntityUpdate{
		ID:              e.ID,
		Name:            e.Name,
		Quantity:        1,
		WarrantyExpires: types.DateFromTime(time.Now().AddDate(0, 0, inDays)),
	})
	require.NoError(t, err)
}

func TestSendExpiryReminders_SendsAggregatedMessage(t *testing.T) {
	mock := newServerChanMock(t, 0)
	withServerChanBaseURL(t, mock.URL)

	setExpiryReminderSettings(t, map[string]interface{}{
		"enabled":     true,
		"sendkey":     "EXPIRYTESTKEY",
		"days_before": []int{17},
		"notify_hour": time.Now().Hour(),
	})

	name := "expiry-" + fk.Str(6)
	createExpiringEntity(t, name, 17)
	createExpiringEntity(t, "expiry-other-"+fk.Str(6), 8) // not on a threshold

	require.NoError(t, tSvc.BackgroundService.SendExpiryReminders(context.Background()))

	pushes := mock.recorded()
	require.Len(t, pushes, 1)
	assert.Equal(t, "/EXPIRYTESTKEY.send", pushes[0].path)
	assert.Contains(t, pushes[0].title, "1 个物品即将到期")
	assert.Contains(t, pushes[0].desp, name)
	assert.Contains(t, pushes[0].desp, "17 天")
	assert.NotContains(t, pushes[0].desp, "expiry-other")
}

func TestSendExpiryReminders_DisabledSendsNothing(t *testing.T) {
	mock := newServerChanMock(t, 0)
	withServerChanBaseURL(t, mock.URL)

	setExpiryReminderSettings(t, map[string]interface{}{
		"enabled":     false,
		"sendkey":     "EXPIRYTESTKEY",
		"days_before": []int{17},
		"notify_hour": time.Now().Hour(),
	})

	require.NoError(t, tSvc.BackgroundService.SendExpiryReminders(context.Background()))
	assert.Empty(t, mock.recorded())
}

func TestSendExpiryReminders_NoSendKeySendsNothing(t *testing.T) {
	mock := newServerChanMock(t, 0)
	withServerChanBaseURL(t, mock.URL)

	setExpiryReminderSettings(t, map[string]interface{}{
		"enabled":     true,
		"sendkey":     "",
		"days_before": []int{17},
		"notify_hour": time.Now().Hour(),
	})

	require.NoError(t, tSvc.BackgroundService.SendExpiryReminders(context.Background()))
	assert.Empty(t, mock.recorded())
}

func TestSendExpiryReminders_WrongHourSendsNothing(t *testing.T) {
	mock := newServerChanMock(t, 0)
	withServerChanBaseURL(t, mock.URL)

	setExpiryReminderSettings(t, map[string]interface{}{
		"enabled":     true,
		"sendkey":     "EXPIRYTESTKEY",
		"days_before": []int{17},
		"notify_hour": (time.Now().Hour() + 1) % 24,
	})

	require.NoError(t, tSvc.BackgroundService.SendExpiryReminders(context.Background()))
	assert.Empty(t, mock.recorded())
}
