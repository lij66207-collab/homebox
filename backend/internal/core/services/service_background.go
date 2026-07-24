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
