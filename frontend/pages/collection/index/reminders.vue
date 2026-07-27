<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import { Button } from "@/components/ui/button";
  import { Label } from "@/components/ui/label";
  import { Switch } from "@/components/ui/switch";
  import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
  import MdiLoading from "~icons/mdi/loading";
  import FormPassword from "~/components/Form/Password.vue";
  import FormCheckbox from "~/components/Form/Checkbox.vue";
  import type { ExpiryReminderSettings } from "~/lib/api/classes/group";

  definePageMeta({
    middleware: ["auth"],
  });

  const { t } = useI18n();

  useHead({ title: `LJJ Organizer | ${t("collection.tabs.reminders")}` });

  const api = useUserApi();
  const { selectedCollection } = useCollections();

  const REDACTED = "REDACTED";

  const loading = ref(true);
  const saving = ref(false);
  const testing = ref(false);

  // The full settings object as received from the server. We only edit the
  // `expiry_reminder` namespace and PUT the whole object back so other
  // namespaces (e.g. `assistant`) are preserved untouched.
  const rawSettings = ref<Record<string, unknown>>({});

  const enabled = ref(false);
  const sendkey = ref("");
  const sendkeyConfigured = ref(false);
  const dayOptions = [30, 7, 3, 1];
  const daysSelected = reactive<Record<number, boolean>>({ 30: true, 7: true, 3: false, 1: true });
  const notifyHour = ref("8");

  const hours = Array.from({ length: 24 }, (_, h) => h.toString());

  const loadSettings = async () => {
    if (!selectedCollection.value) {
      loading.value = false;
      return;
    }

    loading.value = true;

    try {
      const res = await api.group.getSettings();
      if (res.error || !res.data) {
        toast.error(t("reminders.toast.load_failed"));
        return;
      }

      rawSettings.value = res.data.item ?? {};

      const ns = (rawSettings.value.expiry_reminder ?? {}) as Partial<ExpiryReminderSettings>;
      enabled.value = ns.enabled === true;
      sendkey.value = "";
      sendkeyConfigured.value = typeof ns.sendkey === "string" && ns.sendkey === REDACTED;
      const days = Array.isArray(ns.days_before) ? ns.days_before : [];
      for (const d of dayOptions) {
        daysSelected[d] = days.includes(d);
      }
      notifyHour.value = typeof ns.notify_hour === "number" ? String(ns.notify_hour) : "8";
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      toast.error(msg);
    } finally {
      loading.value = false;
    }
  };

  watch(
    () => selectedCollection.value?.id,
    () => {
      void loadSettings();
    },
    { immediate: true }
  );

  const save = async () => {
    if (!selectedCollection.value) return;

    saving.value = true;

    try {
      const previous = (rawSettings.value.expiry_reminder ?? {}) as Partial<ExpiryReminderSettings>;
      const next: ExpiryReminderSettings = {
        enabled: enabled.value,
        // An empty input keeps the stored key: echo back whatever the server
        // gave us ("REDACTED" keeps the value, "" means none configured).
        sendkey: sendkey.value !== "" ? sendkey.value : ((previous.sendkey as string | undefined) ?? ""),
        days_before: dayOptions.filter(d => daysSelected[d]),
        notify_hour: parseInt(notifyHour.value, 10),
      };

      const res = await api.group.updateSettings({
        ...rawSettings.value,
        expiry_reminder: next,
      });

      if (res.error || !res.data) {
        toast.error(t("reminders.toast.save_failed"));
        return;
      }

      rawSettings.value = res.data.item ?? {};
      sendkey.value = "";
      sendkeyConfigured.value =
        typeof rawSettings.value.expiry_reminder === "object" &&
        (rawSettings.value.expiry_reminder as Partial<ExpiryReminderSettings>).sendkey === REDACTED;
      toast.success(t("reminders.toast.saved"));
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      toast.error(msg);
    } finally {
      saving.value = false;
    }
  };

  const sendTest = async () => {
    testing.value = true;
    try {
      // When the input is empty the server falls back to the stored SendKey.
      const res = await api.group.testServerChan(sendkey.value);
      if (res.error) {
        toast.error(t("reminders.toast.test_failed"));
        return;
      }
      toast.success(t("reminders.toast.test_sent"));
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      toast.error(msg);
    } finally {
      testing.value = false;
    }
  };
</script>

<template>
  <div class="space-y-4">
    <div v-if="loading" class="rounded-md border bg-card p-4 text-sm text-muted-foreground">
      {{ $t("global.loading") }}
    </div>

    <div v-else>
      <div v-if="!selectedCollection" class="rounded-md border bg-card p-4 text-sm text-muted-foreground">
        {{ $t("components.collection.selector.select_collection") }}
      </div>

      <div v-else class="space-y-4 rounded-md border bg-card p-4">
        <p class="text-sm text-muted-foreground">{{ $t("reminders.description") }}</p>

        <div class="flex items-center gap-2">
          <Switch id="expiry-reminder-enabled" v-model="enabled" />
          <Label for="expiry-reminder-enabled">{{ $t("reminders.enabled") }}</Label>
        </div>

        <FormPassword
          v-model="sendkey"
          :label="$t('reminders.sendkey')"
          :placeholder="
            sendkeyConfigured ? $t('reminders.sendkey_configured_placeholder') : $t('reminders.sendkey_placeholder')
          "
        />

        <div>
          <Label>{{ $t("reminders.days_before") }}</Label>
          <div class="mt-2 flex flex-wrap gap-4">
            <FormCheckbox
              v-for="d in dayOptions"
              :key="d"
              v-model="daysSelected[d]"
              :label="$t('reminders.days_option', { days: d })"
            />
          </div>
        </div>

        <div>
          <Label for="expiry-reminder-hour">{{ $t("reminders.notify_hour") }}</Label>
          <Select id="expiry-reminder-hour" v-model="notifyHour">
            <SelectTrigger class="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="h in hours" :key="h" :value="h">{{ h.padStart(2, "0") }}:00</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="mt-4 flex items-center gap-2">
          <Button variant="secondary" size="sm" :disabled="saving" @click="save">
            <MdiLoading v-if="saving" class="mr-2 inline-block animate-spin" />
            <span>{{ $t("reminders.save") }}</span>
          </Button>
          <Button variant="outline" size="sm" :disabled="testing" @click="sendTest">
            <MdiLoading v-if="testing" class="mr-2 inline-block animate-spin" />
            <span>{{ $t("reminders.send_test") }}</span>
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>
