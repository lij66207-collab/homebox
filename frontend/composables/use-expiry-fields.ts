import { ref, watch, type Ref } from "vue";
import { isDateOnlyString, parseDateOnly, toDateOnlyString } from "~/lib/datelib/dateOnly";

export type ExpiryStatus = "ok" | "near" | "expired";

export interface ExpiryFieldRefs {
  productionDate: Ref<string>;
  shelfLifeDays: Ref<number | null>;
  expiryDate: Ref<string>;
}

const DAY_MS = 86_400_000;

function addDays(date: string, days: number): string {
  const d = parseDateOnly(date);
  if (!d) return "";
  d.setDate(d.getDate() + days);
  return toDateOnlyString(d);
}

function diffDays(from: string, to: string): number | null {
  const a = parseDateOnly(from);
  const b = parseDateOnly(to);
  if (!a || !b) return null;
  return Math.round((b.getTime() - a.getTime()) / DAY_MS);
}

/**
 * Pure derivation helper mirroring the backend: productionDate + shelfLifeDays
 * -> expiryDate, productionDate + expiryDate -> shelfLifeDays. Only *missing*
 * values are derived (an empty/invalid expiry date counts as missing, as does
 * a null or non-positive shelf life); supplied values pass through untouched,
 * and nothing is derived without a valid production date.
 */
export function deriveExpiry(
  productionDate: string,
  shelfLifeDays: number | null | undefined,
  expiryDate: string
): { shelfLifeDays: number | null; expiryDate: string } {
  let days = shelfLifeDays ?? null;
  let expiry = expiryDate;

  if (isDateOnlyString(productionDate)) {
    if ((days === null || days <= 0) && isDateOnlyString(expiry)) {
      days = diffDays(productionDate, expiry);
    }
    if (!isDateOnlyString(expiry) && days !== null && days > 0) {
      expiry = addDays(productionDate, days);
    }
  }

  return { shelfLifeDays: days, expiryDate: expiry };
}

/**
 * Reactive trio of expiry fields with live derivation, mirroring the backend:
 * productionDate + shelfLifeDays -> expiryDate, productionDate + expiryDate ->
 * shelfLifeDays. Derivation keeps writing into a target field while it is
 * empty OR still holds the last auto-derived value; once the user manually
 * edits a target, auto-derivation stops for that field until it is cleared,
 * so a user-typed value is never overwritten. Nothing happens without a
 * production date.
 *
 * Pass existing refs (e.g. `toRef(form, "productionDate")` or computed
 * proxies into a loaded entity) to wire the derivation onto them; omit an
 * entry to have the composable create a standalone ref.
 */
export function useExpiryFields(fields?: Partial<ExpiryFieldRefs>): ExpiryFieldRefs {
  const productionDate = fields?.productionDate ?? ref("");
  const shelfLifeDays = fields?.shelfLifeDays ?? ref<number | null>(null);
  const expiryDate = fields?.expiryDate ?? ref("");

  // The last value auto-derivation wrote into each target (null = never
  // derived). A target that is non-empty and differs from this marker was
  // manually edited and is left alone; an empty target may be (re-)derived.
  let lastDerivedExpiry: string | null = null;
  let lastDerivedShelfLife: number | null = null;

  watch([productionDate, shelfLifeDays, expiryDate], () => {
    if (!isDateOnlyString(productionDate.value)) return;

    // productionDate + shelfLifeDays -> expiryDate
    if (!expiryDate.value || expiryDate.value === lastDerivedExpiry) {
      if (typeof shelfLifeDays.value === "number" && shelfLifeDays.value > 0) {
        const next = addDays(productionDate.value, shelfLifeDays.value);
        lastDerivedExpiry = next;
        if (next !== expiryDate.value) expiryDate.value = next;
      }
    }

    // productionDate + expiryDate -> shelfLifeDays
    const current = shelfLifeDays.value;
    if (current === null || current === undefined || current === lastDerivedShelfLife) {
      if (isDateOnlyString(expiryDate.value)) {
        const next = diffDays(productionDate.value, expiryDate.value);
        lastDerivedShelfLife = next;
        if (next !== current) shelfLifeDays.value = next;
      }
    }
  });

  return { productionDate, shelfLifeDays, expiryDate };
}

/**
 * Whole days from today (local calendar day) until the given YYYY-MM-DD expiry
 * date. Negative when already expired, null when the date is missing/invalid.
 */
export function daysUntilExpiry(expiryDate: string | null | undefined): number | null {
  if (!expiryDate || !isDateOnlyString(expiryDate)) return null;
  const target = parseDateOnly(expiryDate);
  if (!target) return null;
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.round((target.getTime() - today.getTime()) / DAY_MS);
}

/**
 * Classify an expiry date against the near-expiry threshold (days).
 * Returns null when there is no usable expiry date.
 */
export function expiryStatus(
  expiryDate: string | null | undefined,
  thresholdDays: number
): { status: ExpiryStatus; days: number } | null {
  const days = daysUntilExpiry(expiryDate);
  if (days === null) return null;
  if (days < 0) return { status: "expired", days };
  if (days <= thresholdDays) return { status: "near", days };
  return { status: "ok", days };
}
