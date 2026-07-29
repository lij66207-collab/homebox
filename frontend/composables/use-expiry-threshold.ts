import type { ExpiryReminderSettings } from "~/lib/api/classes/group";

export const DEFAULT_EXPIRY_THRESHOLD_DAYS = 30;

// Module-level guard so the settings are fetched once per session no matter
// how many components (item cards, detail pages, search page) ask for the
// threshold.
let fetchStarted = false;

/**
 * The near-expiry threshold in days: the largest `expiry_reminder.days_before`
 * entry from the group settings, falling back to 30 when reminders are unset
 * or disabled. The result is cached in shared state, so repeat callers reuse
 * it without refetching.
 */
export function useExpiryThreshold() {
  const threshold = useState<number>("expiry-threshold-days", () => DEFAULT_EXPIRY_THRESHOLD_DAYS);

  async function refresh() {
    const api = useUserApi();
    try {
      const res = await api.group.getSettings();
      const ns = ((res.data?.item ?? {}).expiry_reminder ?? {}) as Partial<ExpiryReminderSettings>;
      const days = Array.isArray(ns.days_before) ? ns.days_before.filter(d => typeof d === "number" && d > 0) : [];
      threshold.value = ns.enabled === true && days.length > 0 ? Math.max(...days) : DEFAULT_EXPIRY_THRESHOLD_DAYS;
    } catch {
      threshold.value = DEFAULT_EXPIRY_THRESHOLD_DAYS;
    }
  }

  if (!fetchStarted) {
    fetchStarted = true;
    void refresh();
  }

  return { threshold, refresh };
}
