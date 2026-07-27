package services

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/pkgs/hasher"
)

type GroupService struct {
	repos *repo.AllRepos
}

func (svc *GroupService) UpdateGroup(ctx Context, data repo.GroupUpdate) (repo.Group, error) {
	if data.Name == "" {
		return repo.Group{}, errors.New("group name cannot be empty")
	}

	if data.Currency == "" {
		return repo.Group{}, errors.New("currency cannot be empty")
	}

	return svc.repos.Groups.GroupUpdate(ctx.Context, ctx.GID, data)
}

func (svc *GroupService) CreateGroup(ctx Context, name string) (repo.Group, error) {
	if name == "" {
		return repo.Group{}, errors.New("group name cannot be empty")
	}

	if ctx.UID == uuid.Nil {
		return repo.Group{}, errors.New("user ID cannot be empty when creating a group")
	}

	group, err := svc.repos.Groups.GroupCreate(ctx.Context, name, ctx.UID)
	if err != nil {
		return repo.Group{}, err
	}

	// Unlike registration, this path doesn't seed default locations/tags, so
	// nothing would lazily create the entity types — leaving the collection
	// unable to create items or locations until a type was added by hand.
	if err := ensureDefaultEntityTypes(ctx.Context, svc.repos, group.ID); err != nil {
		return repo.Group{}, err
	}

	return group, nil
}

func (svc *GroupService) DeleteGroup(ctx Context) error {
	return svc.repos.Groups.GroupDelete(ctx.Context, ctx.GID)
}

// RedactedSettingsValue is the sentinel substituted for sensitive group
// settings values in API responses. It must not match any plausible real
// value; clients echo it back on update to signal "keep the stored value".
const RedactedSettingsValue = "REDACTED"

// Namespaces and keys of the group settings document.
const (
	settingsNamespaceAssistant      = "assistant"
	settingsNamespaceExpiryReminder = "expiry_reminder"
	settingsKeyAssistantEnabled     = "enabled"
	settingsKeySTTBaseURL           = "stt_base_url"
	settingsKeySTTAPIKey            = "stt_api_key"
	settingsKeySTTModel             = "stt_model"
	settingsKeySendkey              = "sendkey"
)

// sensitiveGroupSettingsKeys are the namespaced keys inside the group
// settings document whose values are secrets. They are redacted on read and
// preserved on write when the client echoes back the redaction sentinel.
var sensitiveGroupSettingsKeys = []struct {
	namespace string
	key       string
}{
	{settingsNamespaceAssistant, settingsKeySTTAPIKey},
	{settingsNamespaceExpiryReminder, settingsKeySendkey},
}

// RedactGroupSettings returns a copy of settings with every sensitive value
// replaced by RedactedSettingsValue. Empty/absent values stay as-is so
// clients can distinguish "not configured" from "configured".
func RedactGroupSettings(settings map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(settings))
	for k, v := range settings {
		out[k] = v
	}

	for _, sk := range sensitiveGroupSettingsKeys {
		ns, ok := out[sk.namespace].(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := ns[sk.key].(string); ok && v != "" {
			// Copy the nested map so the caller's settings are not mutated.
			redacted := make(map[string]interface{}, len(ns))
			for k, v := range ns {
				redacted[k] = v
			}
			redacted[sk.key] = RedactedSettingsValue
			out[sk.namespace] = redacted
		}
	}

	return out
}

// GetSettings returns the group's raw settings document, including secrets.
// API handlers must pass the result through RedactGroupSettings.
func (svc *GroupService) GetSettings(ctx Context) (map[string]interface{}, error) {
	return svc.repos.Groups.GetSettings(ctx.Context, ctx.GID)
}

// GetServerChanSendKey returns the group's stored Server酱 sendkey, or ""
// when none is configured.
func (svc *GroupService) GetServerChanSendKey(ctx Context) (string, error) {
	settings, err := svc.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	ns, ok := settings[settingsNamespaceExpiryReminder].(map[string]interface{})
	if !ok {
		return "", nil
	}
	sendKey, _ := ns[settingsKeySendkey].(string)
	return sendKey, nil
}

// SetSettings replaces the group's settings document wholesale. Sensitive
// keys submitted as RedactedSettingsValue or an empty string keep their
// currently stored value; every other key is stored as submitted.
func (svc *GroupService) SetSettings(ctx Context, settings map[string]interface{}) (map[string]interface{}, error) {
	current, err := svc.repos.Groups.GetSettings(ctx.Context, ctx.GID)
	if err != nil {
		return nil, err
	}

	for _, sk := range sensitiveGroupSettingsKeys {
		ns, ok := settings[sk.namespace].(map[string]interface{})
		if !ok {
			continue
		}
		v, ok := ns[sk.key].(string)
		if !ok || (v != "" && v != RedactedSettingsValue) {
			continue
		}

		stored := ""
		if curNS, ok := current[sk.namespace].(map[string]interface{}); ok {
			stored, _ = curNS[sk.key].(string)
		}
		if stored != "" {
			ns[sk.key] = stored
		} else {
			delete(ns, sk.key)
		}
	}

	if err := svc.repos.Groups.UpdateSettings(ctx.Context, ctx.GID, settings); err != nil {
		return nil, err
	}

	return settings, nil
}

func (svc *GroupService) NewInvitation(ctx Context, uses int, expiresAt time.Time) (repo.GroupInvitation, string, error) {
	token := hasher.GenerateToken()

	invitation, err := svc.repos.Groups.InvitationCreate(ctx, ctx.GID, repo.GroupInvitationCreate{
		Token:     token.Hash,
		Uses:      uses,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return repo.GroupInvitation{}, "", err
	}

	return invitation, token.Raw, nil
}

func (svc *GroupService) RemoveMember(ctx Context, userID uuid.UUID) error {
	if userID == uuid.Nil {
		return errors.New("user ID cannot be empty")
	}

	err := svc.repos.Groups.RemoveMember(ctx.Context, ctx.GID, userID)
	if err != nil {
		return err
	}

	// If the removed group was the user's default group, reassign to another group
	removedUser, err := svc.repos.Users.GetOneID(ctx.Context, userID)
	if err != nil {
		return err
	}

	if removedUser.DefaultGroupID == ctx.GID {
		// Find another group the user is still a member of
		var newDefaultGroupID uuid.UUID
		for _, gid := range removedUser.GroupIDs {
			if gid != ctx.GID {
				newDefaultGroupID = gid
				break
			}
		}
		// Update to another group, or uuid.Nil if the user has no remaining groups
		if err := svc.repos.Users.UpdateDefaultGroup(ctx.Context, userID, newDefaultGroupID); err != nil {
			return err
		}
	}

	return nil
}

func (svc *GroupService) DeleteInvitation(ctx Context, id uuid.UUID) error {
	return svc.repos.Groups.InvitationDelete(ctx.Context, ctx.GID, id)
}

func (svc *GroupService) AcceptInvitation(ctx Context, token string) (repo.Group, error) {
	hashedToken := hasher.HashToken(token)
	return svc.repos.Groups.InvitationAccept(ctx.Context, hashedToken, ctx.UID)
}
