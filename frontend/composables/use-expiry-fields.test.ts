import { describe, expect, test } from "vitest";
import { nextTick, ref } from "vue";
import { deriveExpiry, useExpiryFields } from "./use-expiry-fields";

describe("deriveExpiry", () => {
  test("derives expiry from production + shelf life", () => {
    expect(deriveExpiry("2026-07-01", 14, "")).toEqual({ shelfLifeDays: 14, expiryDate: "2026-07-15" });
  });

  test("derives shelf life from production + expiry", () => {
    expect(deriveExpiry("2026-07-01", null, "2026-07-15")).toEqual({ shelfLifeDays: 14, expiryDate: "2026-07-15" });
  });

  test("keeps supplied values untouched", () => {
    expect(deriveExpiry("2026-07-01", 10, "2026-07-15")).toEqual({ shelfLifeDays: 10, expiryDate: "2026-07-15" });
  });

  test("derives nothing without a production date", () => {
    expect(deriveExpiry("", 14, "")).toEqual({ shelfLifeDays: 14, expiryDate: "" });
    expect(deriveExpiry("", null, "2026-07-15")).toEqual({ shelfLifeDays: null, expiryDate: "2026-07-15" });
  });

  test("ignores non-positive shelf life and invalid expiry", () => {
    expect(deriveExpiry("2026-07-01", 0, "")).toEqual({ shelfLifeDays: 0, expiryDate: "" });
    expect(deriveExpiry("2026-07-01", null, "not-a-date")).toEqual({ shelfLifeDays: null, expiryDate: "not-a-date" });
  });
});

describe("useExpiryFields", () => {
  test("re-derives expiry while it still holds the last auto-derived value", async () => {
    const productionDate = ref("2026-07-01");
    const shelfLifeDays = ref<number | null>(null);
    const expiryDate = ref("");
    useExpiryFields({ productionDate, shelfLifeDays, expiryDate });

    shelfLifeDays.value = 1;
    await nextTick();
    expect(expiryDate.value).toBe("2026-07-02");

    // Continue typing "12": expiry must follow to +12 days.
    shelfLifeDays.value = 12;
    await nextTick();
    expect(expiryDate.value).toBe("2026-07-13");
  });

  test("stops deriving into a manually edited target until it is cleared", async () => {
    const productionDate = ref("2026-07-01");
    const shelfLifeDays = ref<number | null>(null);
    const expiryDate = ref("");
    useExpiryFields({ productionDate, shelfLifeDays, expiryDate });

    shelfLifeDays.value = 1;
    await nextTick();
    expect(expiryDate.value).toBe("2026-07-02");

    // Manual edit breaks the auto-derive link for expiry only.
    expiryDate.value = "2026-08-01";
    await nextTick();
    shelfLifeDays.value = 12;
    await nextTick();
    expect(expiryDate.value).toBe("2026-08-01");

    // Clearing the field re-enables derivation.
    expiryDate.value = "";
    await nextTick();
    expect(expiryDate.value).toBe("2026-07-13");
  });

  test("derives shelf life from production + expiry and stops on manual edit", async () => {
    const productionDate = ref("2026-07-01");
    const shelfLifeDays = ref<number | null>(null);
    const expiryDate = ref("");
    useExpiryFields({ productionDate, shelfLifeDays, expiryDate });

    expiryDate.value = "2026-07-15";
    await nextTick();
    expect(shelfLifeDays.value).toBe(14);

    expiryDate.value = "2026-07-31";
    await nextTick();
    expect(shelfLifeDays.value).toBe(30);

    shelfLifeDays.value = 99;
    await nextTick();
    expiryDate.value = "2026-08-15";
    await nextTick();
    expect(shelfLifeDays.value).toBe(99);

    shelfLifeDays.value = null;
    await nextTick();
    expect(shelfLifeDays.value).toBe(45);
  });

  test("does nothing without a production date", async () => {
    const productionDate = ref("");
    const shelfLifeDays = ref<number | null>(12);
    const expiryDate = ref("");
    useExpiryFields({ productionDate, shelfLifeDays, expiryDate });

    await nextTick();
    expect(expiryDate.value).toBe("");
  });
});
