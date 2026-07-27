<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import { Button } from "@/components/ui/button";
  import { Label } from "@/components/ui/label";
  import { Switch } from "@/components/ui/switch";
  import MdiLoading from "~icons/mdi/loading";
  import MdiAlert from "~icons/mdi/alert";
  import FormTextField from "~/components/Form/TextField.vue";
  import FormPassword from "~/components/Form/Password.vue";
  import type { AssistantSettings } from "~/lib/api/classes/group";

  definePageMeta({
    middleware: ["auth"],
  });

  const { t } = useI18n();

  useHead({ title: `LJJ Organizer | ${t("collection.tabs.assistant")}` });

  const api = useUserApi();
  const { selectedCollection } = useCollections();

  // The global AI flag lives on the public status endpoint; the assistant also
  // needs it (intent parsing) in addition to the per-collection STT config.
  const pubApi = usePublicApi();
  const { data: status } = useAsyncData(async () => {
    const { data } = await pubApi.status();
    return data;
  });
  const aiEnabled = computed(() => status.value?.aiEnabled === true);

  const REDACTED = "REDACTED";

  const loading = ref(true);
  const saving = ref(false);

  // The full settings object as received from the server. We only edit the
  // `assistant` namespace and PUT the whole object back so other namespaces
  // (e.g. `expiry_reminder`) are preserved untouched.
  const rawSettings = ref<Record<string, unknown>>({});

  const enabled = ref(false);
  const sttBaseUrl = ref("");
  const sttApiKey = ref("");
  const sttApiKeyConfigured = ref(false);
  const sttModel = ref("");

  const loadSettings = async () => {
    if (!selectedCollection.value) {
      loading.value = false;
      return;
    }

    loading.value = true;

    try {
      const res = await api.group.getSettings();
      if (res.error || !res.data) {
        toast.error(t("assistant.settings.toast.load_failed"));
        return;
      }

      rawSettings.value = res.data.item ?? {};

      const ns = (rawSettings.value.assistant ?? {}) as Partial<AssistantSettings>;
      enabled.value = ns.enabled === true;
      sttBaseUrl.value = typeof ns.stt_base_url === "string" ? ns.stt_base_url : "";
      sttApiKey.value = "";
      sttApiKeyConfigured.value = typeof ns.stt_api_key === "string" && ns.stt_api_key === REDACTED;
      sttModel.value = typeof ns.stt_model === "string" ? ns.stt_model : "";
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
      const previous = (rawSettings.value.assistant ?? {}) as Partial<AssistantSettings>;
      const next: AssistantSettings = {
        enabled: enabled.value,
        stt_base_url: sttBaseUrl.value.trim(),
        // An empty input keeps the stored key: echo back whatever the server
        // gave us ("REDACTED" keeps the value, "" means none configured).
        stt_api_key: sttApiKey.value !== "" ? sttApiKey.value : ((previous.stt_api_key as string | undefined) ?? ""),
        stt_model: sttModel.value.trim(),
      };

      const res = await api.group.updateSettings({
        ...rawSettings.value,
        assistant: next,
      });

      if (res.error || !res.data) {
        toast.error(t("assistant.settings.toast.save_failed"));
        return;
      }

      rawSettings.value = res.data.item ?? {};
      sttApiKey.value = "";
      sttApiKeyConfigured.value =
        typeof rawSettings.value.assistant === "object" &&
        (rawSettings.value.assistant as Partial<AssistantSettings>).stt_api_key === REDACTED;
      toast.success(t("assistant.settings.toast.saved"));
    } catch (e) {
      const msg = (e as Error).message ?? String(e);
      toast.error(msg);
    } finally {
      saving.value = false;
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
        <div
          v-if="!aiEnabled"
          class="flex items-start gap-2 rounded-lg border-l-4 border-l-destructive bg-destructive/5 p-3 text-sm"
        >
          <MdiAlert class="mt-0.5 size-4 shrink-0 text-destructive" />
          <span>{{ $t("assistant.settings.ai_disabled_hint") }}</span>
        </div>

        <p class="text-sm text-muted-foreground">{{ $t("assistant.settings.description") }}</p>

        <div class="flex items-center gap-2">
          <Switch id="assistant-enabled" v-model="enabled" />
          <Label for="assistant-enabled">{{ $t("assistant.settings.enabled") }}</Label>
        </div>

        <div>
          <FormTextField
            v-model="sttBaseUrl"
            :label="$t('assistant.settings.stt_base_url')"
            placeholder="https://api.openai.com/v1"
          />
          <p class="m-2 text-sm text-muted-foreground">{{ $t("assistant.settings.stt_base_url_hint") }}</p>
        </div>

        <FormPassword
          v-model="sttApiKey"
          :label="$t('assistant.settings.stt_api_key')"
          :placeholder="
            sttApiKeyConfigured
              ? $t('assistant.settings.stt_api_key_configured_placeholder')
              : $t('assistant.settings.stt_api_key_placeholder')
          "
        />

        <FormTextField v-model="sttModel" :label="$t('assistant.settings.stt_model')" placeholder="whisper-1" />

        <div class="mt-4">
          <Button variant="secondary" size="sm" :disabled="saving" @click="save">
            <MdiLoading v-if="saving" class="mr-2 inline-block animate-spin" />
            <span>{{ $t("assistant.settings.save") }}</span>
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>
