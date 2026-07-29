package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nicholas-fedor/shoutrrr"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

type Latest struct {
	Version string `json:"version"`
	Date    string `json:"date"`
}
type BackgroundService struct {
	repos          *repo.AllRepos
	latest         Latest
	notifierConfig *config.NotifierConf
}

func (svc *BackgroundService) SendNotifiersToday(ctx context.Context) error {
	// Get All Groups
	groups, err := svc.repos.Groups.GetAllGroups(ctx, uuid.Nil)
	if err != nil {
		return err
	}

	today := types.DateFromTime(time.Now())

	for i := range groups {
		group := groups[i]

		entries, err := svc.repos.MaintEntry.GetScheduled(ctx, group.ID, today)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			log.Debug().
				Str("group_name", group.Name).
				Str("group_id", group.ID.String()).
				Msg("No scheduled maintenance for today")
			continue
		}

		notifiers, err := svc.repos.Notifiers.GetActiveByGroup(ctx, group.ID)
		if err != nil {
			return err
		}

		if len(notifiers) == 0 {
			log.Debug().
				Str("group_name", group.Name).
				Str("group_id", group.ID.String()).
				Msg("No active notifiers configured")
			continue
		}

		bldr := strings.Builder{}

		bldr.WriteString("Homebox Maintenance for (")
		bldr.WriteString(today.String())
		bldr.WriteString("):\n")

		for i := range entries {
			entry := entries[i]
			bldr.WriteString(" - ")
			bldr.WriteString(entry.Name)
			bldr.WriteString("\n")
		}

		var sendErrs []error
		for i := range notifiers {
			// Validate notifier URL before sending
			if err := validate.ValidateNotifierURL(notifiers[i].URL, svc.notifierConfig); err != nil {
				log.Error().
					Err(err).
					Str("notifier_id", notifiers[i].ID.String()).
					Str("notifier_name", notifiers[i].Name).
					Msg("notifier URL failed validation, skipping")
				sendErrs = append(sendErrs, fmt.Errorf("notifier %s failed validation: %w", notifiers[i].Name, err))
				continue
			}

			err := shoutrrr.Send(notifiers[i].URL, bldr.String())

			if err != nil {
				sendErrs = append(sendErrs, err)
			}
		}

		if len(sendErrs) > 0 {
			return sendErrs[0]
		}
	}

	return nil
}

func (svc *BackgroundService) GetLatestGithubRelease(ctx context.Context) error {
	url := "https://api.github.com/repos/sysadminsmedia/homebox/releases/latest"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create latest version request: %w", err)
	}

	req.Header.Set("User-Agent", "Homebox-Version-Checker")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make latest version request: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing latest version response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("latest version unexpected status code: %d", resp.StatusCode)
	}

	// ignoring fields that are not relevant
	type Release struct {
		ReleaseVersion string    `json:"tag_name"`
		PublishedAt    time.Time `json:"published_at"`
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode latest version response: %w", err)
	}

	svc.latest = Latest{
		Version: release.ReleaseVersion,
		Date:    release.PublishedAt.String(),
	}

	return nil
}

func (svc *BackgroundService) GetLatestVersion() Latest {
	return svc.latest
}

// sendToGroupNotifiers delivers a message to all active notifiers of a group.
// Notifiers whose URL fails SSRF validation are skipped and logged.
func (svc *BackgroundService) sendToGroupNotifiers(ctx context.Context, gid uuid.UUID, message string) error {
	notifiers, err := svc.repos.Notifiers.GetActiveByGroup(ctx, gid)
	if err != nil {
		return err
	}

	var sendErrs []error
	for i := range notifiers {
		if err := validate.ValidateNotifierURL(notifiers[i].URL, svc.notifierConfig); err != nil {
			log.Error().
				Err(err).
				Str("notifier_id", notifiers[i].ID.String()).
				Str("notifier_name", notifiers[i].Name).
				Msg("notifier URL failed validation, skipping")
			sendErrs = append(sendErrs, fmt.Errorf("notifier %s failed validation: %w", notifiers[i].Name, err))
			continue
		}

		if err := shoutrrr.Send(notifiers[i].URL, message); err != nil {
			sendErrs = append(sendErrs, err)
		}
	}

	if len(sendErrs) > 0 {
		return sendErrs[0]
	}
	return nil
}

// warrantyReminderDays are the day-offsets before warranty expiry that
// trigger a reminder: 30 days out, 7 days out, and on the day itself.
var warrantyReminderDays = []int{30, 7, 0}

// SendWarrantyReminders notifies each group about items whose warranty
// expires in 30 days, 7 days, or today. Runs daily; items are reported at
// most three times (once per touchpoint).
func (svc *BackgroundService) SendWarrantyReminders(ctx context.Context) error {
	groups, err := svc.repos.Groups.GetAllGroups(ctx, uuid.Nil)
	if err != nil {
		return err
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	until := today.AddDate(0, 0, warrantyReminderDays[0]+1)

	for i := range groups {
		items, err := svc.repos.Entities.GetWarrantyExpiringBetween(ctx, groups[i].ID, today, until)
		if err != nil {
			return err
		}

		var lines []string
		for _, item := range items {
			expires := item.WarrantyExpires.Time()
			expiresDay := time.Date(expires.Year(), expires.Month(), expires.Day(), 0, 0, 0, 0, expires.Location())
			days := int(expiresDay.Sub(today).Hours() / 24)
			for _, d := range warrantyReminderDays {
				if days == d {
					lines = append(lines, fmt.Sprintf(" - %s (expires %s, in %d days)", item.Name, expiresDay.Format("2006-01-02"), days))
					break
				}
			}
		}

		if len(lines) == 0 {
			continue
		}

		bldr := strings.Builder{}
		bldr.WriteString("Homebox warranty reminders:\n")
		for _, line := range lines {
			bldr.WriteString(line)
			bldr.WriteString("\n")
		}

		if err := svc.sendToGroupNotifiers(ctx, groups[i].ID, bldr.String()); err != nil {
			return err
		}
	}

	return nil
}

// defaultExpiryReminderDays are the day-offsets before expiry that trigger a
// Server酱 reminder when the group has not configured its own thresholds.
var defaultExpiryReminderDays = []int{30, 7, 1}

// defaultExpiryNotifyHour is the hour of day (local server time) at which
// expiry reminders are sent unless the group configures notify_hour.
const defaultExpiryNotifyHour = 8

// expiryReminderConfig is the parsed `expiry_reminder` namespace of a group's
// settings document.
type expiryReminderConfig struct {
	enabled    bool
	sendKey    string
	daysBefore []int
	notifyHour int
}

// parseExpiryReminderConfig extracts the expiry reminder configuration from a
// raw group settings document, applying defaults for absent values. Numbers
// arrive as float64 because the document round-trips through JSON.
func parseExpiryReminderConfig(settings map[string]interface{}) expiryReminderConfig {
	cfg := expiryReminderConfig{
		daysBefore: defaultExpiryReminderDays,
		notifyHour: defaultExpiryNotifyHour,
	}

	ns, ok := settings[settingsNamespaceExpiryReminder].(map[string]interface{})
	if !ok {
		return cfg
	}

	cfg.enabled, _ = ns["enabled"].(bool)
	cfg.sendKey, _ = ns[settingsKeySendkey].(string)

	if v, ok := ns["notify_hour"].(float64); ok {
		cfg.notifyHour = int(v)
	}

	if raw, ok := ns["days_before"].([]interface{}); ok {
		days := make([]int, 0, len(raw))
		for _, d := range raw {
			if f, ok := d.(float64); ok && f >= 0 {
				days = append(days, int(f))
			}
		}
		if len(days) > 0 {
			cfg.daysBefore = days
		}
	}

	return cfg
}

// SendExpiryReminders pushes one aggregated Server酱 message per group about
// items whose expiry date (the expiry_date field, 截止日期) is exactly N
// days away, with N taken from the group's `expiry_reminder.days_before`
// setting (default 30/7/1). A group is only considered when the reminder is
// enabled, a sendkey is configured, and the current local hour matches the
// group's notify_hour (default 8) — the recurring task calls this every hour.
// A failure for one group is logged and does not stop the others.
func (svc *BackgroundService) SendExpiryReminders(ctx context.Context) error {
	groups, err := svc.repos.Groups.GetAllGroups(ctx, uuid.Nil)
	if err != nil {
		return err
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var firstErr error
	for i := range groups {
		group := groups[i]
		logger := log.With().
			Str("group_name", group.Name).
			Str("group_id", group.ID.String()).
			Logger()

		settings, err := svc.repos.Groups.GetSettings(ctx, group.ID)
		if err != nil {
			logger.Err(err).Msg("expiry reminder: failed to load group settings")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		cfg := parseExpiryReminderConfig(settings)
		if !cfg.enabled || cfg.sendKey == "" {
			continue
		}
		if now.Hour() != cfg.notifyHour {
			continue
		}

		maxDays := 0
		thresholds := make(map[int]struct{}, len(cfg.daysBefore))
		for _, d := range cfg.daysBefore {
			thresholds[d] = struct{}{}
			if d > maxDays {
				maxDays = d
			}
		}

		items, err := svc.repos.Entities.GetExpiringBetween(ctx, group.ID, today, today.AddDate(0, 0, maxDays+1))
		if err != nil {
			logger.Err(err).Msg("expiry reminder: failed to query expiring items")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		bldr := strings.Builder{}
		bldr.WriteString("| 物品 | 到期日期 | 剩余天数 |\n| --- | --- | --- |\n")
		count := 0
		for _, item := range items {
			expires := item.ExpiryDate.Time()
			expiresDay := time.Date(expires.Year(), expires.Month(), expires.Day(), 0, 0, 0, 0, expires.Location())
			days := int(expiresDay.Sub(today).Hours() / 24)
			if _, ok := thresholds[days]; !ok {
				continue
			}
			fmt.Fprintf(&bldr, "| %s | %s | %d 天 |\n", item.Name, expiresDay.Format("2006-01-02"), days)
			count++
		}

		if count == 0 {
			continue
		}

		title := fmt.Sprintf("Homebox 保质期提醒：%d 个物品即将到期", count)
		if err := SendServerChan(ctx, cfg.sendKey, title, bldr.String()); err != nil {
			// Never log the sendkey itself.
			logger.Err(err).Msg("expiry reminder: serverchan push failed")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		logger.Info().Int("items", count).Msg("expiry reminder: serverchan push sent")
	}

	return firstErr
}

// SendLowStockReminders notifies each group about items whose quantity has
// dropped to or below their configured low-stock threshold. Each item is
// reported once per depletion cycle — the latch resets when the quantity
// rises above the threshold again.
func (svc *BackgroundService) SendLowStockReminders(ctx context.Context) error {
	groups, err := svc.repos.Groups.GetAllGroups(ctx, uuid.Nil)
	if err != nil {
		return err
	}

	for i := range groups {
		items, err := svc.repos.Entities.GetLowStock(ctx, groups[i].ID)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			continue
		}

		bldr := strings.Builder{}
		bldr.WriteString("Homebox low stock reminders:\n")

		ids := make([]uuid.UUID, 0, len(items))
		for _, item := range items {
			bldr.WriteString(fmt.Sprintf(" - %s (quantity %g, threshold %g)\n", item.Name, item.Quantity, *item.LowStockThreshold))
			ids = append(ids, item.ID)
		}

		if err := svc.sendToGroupNotifiers(ctx, groups[i].ID, bldr.String()); err != nil {
			return err
		}

		if err := svc.repos.Entities.MarkLowStockNotified(ctx, groups[i].ID, ids); err != nil {
			return err
		}
	}

	return nil
}
