package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const settingsKeyEnabled = "enabled"

func settingsNamespace(t *testing.T, settings map[string]interface{}, namespace string) map[string]interface{} {
	t.Helper()
	ns, ok := settings[namespace].(map[string]interface{})
	require.True(t, ok, "settings should contain a %q object", namespace)
	return ns
}

func TestGroupService_SettingsRoundTrip(t *testing.T) {
	settings := map[string]interface{}{
		settingsNamespaceAssistant: map[string]interface{}{
			settingsKeyEnabled:   true,
			settingsKeySTTAPIKey: "sk-test-key",
			"stt_model":          "whisper-1",
		},
		settingsNamespaceExpiryReminder: map[string]interface{}{
			settingsKeyEnabled: true,
			settingsKeySendkey: "sct-test-key",
		},
	}

	saved, err := tSvc.Group.SetSettings(tCtx, settings)
	require.NoError(t, err)
	assert.Equal(t, settings, saved)

	got, err := tSvc.Group.GetSettings(tCtx)
	require.NoError(t, err)
	assert.Equal(t, settings, got)
}

func TestGroupService_SetSettingsPreservesRedactedSecrets(t *testing.T) {
	_, err := tSvc.Group.SetSettings(tCtx, map[string]interface{}{
		settingsNamespaceAssistant:      map[string]interface{}{settingsKeySTTAPIKey: "sk-stored"},
		settingsNamespaceExpiryReminder: map[string]interface{}{settingsKeySendkey: "sct-stored", settingsKeyEnabled: false},
	})
	require.NoError(t, err)

	// The client echoes the redaction sentinel (or an empty string) back for
	// secret fields it did not touch — the stored secrets must survive while
	// every other key updates normally.
	_, err = tSvc.Group.SetSettings(tCtx, map[string]interface{}{
		settingsNamespaceAssistant:      map[string]interface{}{settingsKeySTTAPIKey: RedactedSettingsValue, settingsKeyEnabled: true},
		settingsNamespaceExpiryReminder: map[string]interface{}{settingsKeySendkey: "", settingsKeyEnabled: true},
	})
	require.NoError(t, err)

	got, err := tSvc.Group.GetSettings(tCtx)
	require.NoError(t, err)

	assistant := settingsNamespace(t, got, settingsNamespaceAssistant)
	assert.Equal(t, "sk-stored", assistant[settingsKeySTTAPIKey])
	enabled, ok := assistant[settingsKeyEnabled].(bool)
	require.True(t, ok)
	assert.True(t, enabled)

	reminder := settingsNamespace(t, got, settingsNamespaceExpiryReminder)
	assert.Equal(t, "sct-stored", reminder[settingsKeySendkey])
	enabled, ok = reminder[settingsKeyEnabled].(bool)
	require.True(t, ok)
	assert.True(t, enabled)
}

func TestGroupService_SetSettingsReplacesRealSecrets(t *testing.T) {
	_, err := tSvc.Group.SetSettings(tCtx, map[string]interface{}{
		settingsNamespaceExpiryReminder: map[string]interface{}{settingsKeySendkey: "sct-old"},
	})
	require.NoError(t, err)

	_, err = tSvc.Group.SetSettings(tCtx, map[string]interface{}{
		settingsNamespaceExpiryReminder: map[string]interface{}{settingsKeySendkey: "sct-new"},
	})
	require.NoError(t, err)

	got, err := tSvc.Group.GetSettings(tCtx)
	require.NoError(t, err)
	assert.Equal(t, "sct-new", settingsNamespace(t, got, settingsNamespaceExpiryReminder)[settingsKeySendkey])
}

func TestRedactGroupSettings(t *testing.T) {
	settings := map[string]interface{}{
		settingsNamespaceAssistant: map[string]interface{}{
			settingsKeySTTAPIKey: "sk-secret",
			"stt_model":          "whisper-1",
		},
		settingsNamespaceExpiryReminder: map[string]interface{}{
			settingsKeySendkey: "",
		},
		"other": "value",
	}

	redacted := RedactGroupSettings(settings)

	assistant := settingsNamespace(t, redacted, settingsNamespaceAssistant)
	assert.Equal(t, RedactedSettingsValue, assistant[settingsKeySTTAPIKey])
	assert.Equal(t, "whisper-1", assistant["stt_model"])

	// Empty secrets stay empty so clients can tell "not configured" apart
	// from "configured".
	assert.Empty(t, settingsNamespace(t, redacted, settingsNamespaceExpiryReminder)[settingsKeySendkey])

	assert.Equal(t, "value", redacted["other"])

	// The caller's map must not be mutated.
	assert.Equal(t, "sk-secret", settingsNamespace(t, settings, settingsNamespaceAssistant)[settingsKeySTTAPIKey])
}
