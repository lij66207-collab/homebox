<template>
  <BaseModal :dialog-id="DialogID.Assistant" :title="$t('assistant.dialog.title')" :hide-footer="true">
    <div class="flex min-w-0 flex-col gap-3">
      <p class="text-sm text-muted-foreground">{{ $t("assistant.dialog.hint") }}</p>

      <div ref="scrollArea" class="flex max-h-96 min-h-40 flex-col gap-2 overflow-y-auto rounded-md border p-3">
        <p v-if="entries.length === 0" class="text-sm text-muted-foreground">
          {{ $t("assistant.dialog.empty") }}
        </p>

        <template v-for="(entry, i) in entries" :key="i">
          <div
            class="max-w-[85%] rounded-lg px-3 py-2 text-sm"
            :class="
              entry.role === 'user' ? 'self-end bg-primary text-primary-foreground' : 'self-start bg-muted text-left'
            "
          >
            <p class="whitespace-pre-wrap">{{ entry.text }}</p>

            <!-- Action confirmation cards -->
            <div
              v-for="(action, j) in entry.actions"
              :key="j"
              class="mt-2 rounded-md border bg-card p-2 text-card-foreground"
            >
              <template v-if="action.type === 'create_location' || action.type === 'create_item'">
                <p class="font-medium">
                  {{
                    $t(
                      action.type === "create_location"
                        ? "assistant.dialog.create_location"
                        : "assistant.dialog.create_item"
                    )
                  }}:
                  {{ action.name }}
                </p>
                <p v-if="action.type === 'create_location' && action.parent_path" class="text-muted-foreground">
                  {{ $t("assistant.dialog.location_label") }}: {{ action.parent_path }}
                </p>
                <p v-if="action.type === 'create_item' && action.location_path" class="text-muted-foreground">
                  {{ $t("assistant.dialog.location_label") }}: {{ action.location_path }}
                </p>
                <p v-if="action.type === 'create_item' && action.quantity" class="text-muted-foreground">
                  {{ $t("assistant.dialog.quantity_label") }}: {{ action.quantity }}
                </p>
                <p v-if="action.type === 'create_item' && action.description" class="text-muted-foreground">
                  {{ action.description }}
                </p>

                <div v-if="action.status === 'pending'" class="mt-2 flex justify-end gap-2">
                  <Button size="sm" :disabled="acting" @click="confirmAction(action)">
                    {{ $t("assistant.dialog.confirm") }}
                  </Button>
                  <Button size="sm" variant="outline" :disabled="acting" @click="action.status = 'cancelled'">
                    {{ $t("assistant.dialog.cancel") }}
                  </Button>
                </div>
                <p v-else class="mt-1 text-muted-foreground">
                  {{ action.status === "done" ? $t("assistant.dialog.done") : $t("assistant.dialog.cancelled") }}
                  <template v-if="action.note"> — {{ action.note }}</template>
                </p>
              </template>
            </div>

            <!-- Query results -->
            <div v-if="entry.results" class="mt-2">
              <p class="font-medium">{{ $t("assistant.dialog.query_results") }}:</p>
              <ul v-if="entry.results.length > 0" class="ml-1 list-none">
                <li v-for="r in entry.results" :key="r.kind + r.id">
                  <NuxtLink
                    :to="r.kind === 'item' ? `/item/${r.id}` : `/location/${r.id}`"
                    class="underline"
                    @click="closeDialog(DialogID.Assistant)"
                  >
                    {{ r.name }}
                  </NuxtLink>
                  <span v-if="r.location" class="text-muted-foreground"> ({{ r.location }})</span>
                </li>
              </ul>
              <p v-else class="text-muted-foreground">{{ $t("assistant.dialog.no_results") }}</p>
            </div>
          </div>
        </template>
      </div>

      <div class="flex flex-col items-center gap-1">
        <Button
          size="icon"
          class="size-14 rounded-full"
          :variant="recording ? 'destructive' : 'default'"
          :disabled="processing"
          :aria-label="$t('assistant.dialog.title')"
          @click="toggleRecording"
        >
          <MdiLoading v-if="processing" class="size-6 animate-spin" />
          <MdiStop v-else-if="recording" class="size-6" />
          <MdiMicrophone v-else class="size-6" />
        </Button>
        <p class="min-h-5 text-center text-sm text-muted-foreground">
          <template v-if="recording">{{ $t("assistant.dialog.recording") }}</template>
          <template v-else-if="processing">{{ $t("assistant.dialog.processing") }}</template>
        </p>
      </div>
    </div>
  </BaseModal>
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { toast } from "@/components/ui/sonner";
  import { Button } from "@/components/ui/button";
  import BaseModal from "@/components/App/CreateModal.vue";
  import { useDialog } from "~/components/ui/dialog-provider";
  import { DialogID } from "@/components/ui/dialog-provider/utils";
  import { useLocationStore } from "~~/stores/locations";
  import { useEntityTypeStore } from "~~/stores/entityTypes";
  import type { AssistantHistoryMessage } from "~~/lib/api/classes/assistant";
  import type { AssistantAction, TreeItem } from "~~/lib/api/types/data-contracts";
  import { toDateOnlyString } from "~/lib/datelib/dateOnly";
  import MdiMicrophone from "~icons/mdi/microphone";
  import MdiStop from "~icons/mdi/stop";
  import MdiLoading from "~icons/mdi/loading";

  const { t } = useI18n();
  const { activeDialog, closeDialog } = useDialog();

  const api = useUserApi();
  const locationStore = useLocationStore();
  const entityTypeStore = useEntityTypeStore();

  type ActionState = AssistantAction & {
    status: "pending" | "done" | "cancelled";
    note?: string;
  };

  type QueryResult = {
    id: string;
    kind: "item" | "location";
    location?: string;
    name: string;
  };

  type ChatEntry = {
    role: "user" | "assistant";
    text: string;
    actions?: ActionState[];
    results?: QueryResult[];
  };

  const entries = ref<ChatEntry[]>([]);
  const history: AssistantHistoryMessage[] = [];
  const HISTORY_LIMIT = 10;

  const recording = ref(false);
  const processing = ref(false);
  const acting = ref(false);

  const scrollArea = ref<HTMLDivElement | null>(null);

  // ---------------------------------------------------------------------------
  // Recording (pattern follows composables/use-barcode-detector.ts: getUserMedia
  // + explicit stream teardown)
  // ---------------------------------------------------------------------------

  let stream: MediaStream | null = null;
  let recorder: MediaRecorder | null = null;
  let chunks: Blob[] = [];
  let discardRecording = false;

  async function startRecording() {
    // getUserMedia is only exposed in secure contexts; over plain HTTP the API
    // is missing entirely, so call it out explicitly instead of "unsupported".
    if (window.isSecureContext === false) {
      toast.error(t("assistant.dialog.mic_insecure"));
      return;
    }

    if (!navigator?.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
      toast.error(t("assistant.dialog.mic_unsupported"));
      return;
    }

    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch (err) {
      console.error("[assistant] getUserMedia error:", err);
      toast.error(t("assistant.dialog.mic_denied"));
      return;
    }

    const mimeType = MediaRecorder.isTypeSupported("audio/webm;codecs=opus") ? "audio/webm;codecs=opus" : "";
    recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
    chunks = [];
    discardRecording = false;
    recorder.ondataavailable = e => {
      if (e.data.size > 0) chunks.push(e.data);
    };
    recorder.onstop = onRecorderStop;
    recorder.start();
    recording.value = true;
  }

  function stopRecording(discard = false) {
    discardRecording = discard;
    if (recorder && recorder.state !== "inactive") {
      recorder.stop();
    }
    recording.value = false;
    if (stream) {
      stream.getTracks().forEach(track => track.stop());
      stream = null;
    }
  }

  function onRecorderStop() {
    const type = recorder?.mimeType || "audio/webm";
    const blob = new Blob(chunks, { type });
    recorder = null;
    chunks = [];
    if (discardRecording || blob.size === 0) {
      discardRecording = false;
      return;
    }
    void sendVoice(blob);
  }

  function toggleRecording() {
    if (processing.value) return;
    if (recording.value) {
      stopRecording();
    } else {
      void startRecording();
    }
  }

  // ---------------------------------------------------------------------------
  // Text-to-speech: browser speechSynthesis, silently degrades to text only
  // ---------------------------------------------------------------------------

  function speak(text: string) {
    try {
      if (!("speechSynthesis" in window)) return;
      window.speechSynthesis.cancel();
      const utterance = new SpeechSynthesisUtterance(text);
      utterance.lang = "zh-CN";
      window.speechSynthesis.speak(utterance);
    } catch {
      // no speech support — text bubbles still show everything
    }
  }

  function stopSpeaking() {
    try {
      if ("speechSynthesis" in window) window.speechSynthesis.cancel();
    } catch {
      // ignore
    }
  }

  // ---------------------------------------------------------------------------
  // Voice round-trip
  // ---------------------------------------------------------------------------

  async function sendVoice(blob: Blob) {
    processing.value = true;
    const { data, error, status } = await api.assistant.voice(blob, history);
    processing.value = false;

    if (error || !data) {
      const msg = status === 409 ? t("assistant.dialog.not_configured") : t("assistant.dialog.request_failed");
      toast.error(msg);
      pushEntry({ role: "assistant", text: msg });
      return;
    }

    pushEntry({ role: "user", text: data.transcript });

    const actions: ActionState[] = (data.actions ?? []).map(a => ({ ...a, status: "pending" }));
    const entry: ChatEntry = { role: "assistant", text: data.reply, actions };
    pushEntry(entry);
    speak(data.reply);

    // Query actions are read-only: run them immediately and render the results.
    let historyReply = data.reply;
    for (const action of actions) {
      if (action.type !== "query_item" && action.type !== "query_location") continue;
      action.status = "done";
      const results = await runQueryAction(action);
      entry.results = [...(entry.results ?? []), ...results];
      historyReply +=
        results.length > 0
          ? `\n${t("assistant.dialog.query_results")}: ${results.map(r => r.name).join(", ")}`
          : `\n${t("assistant.dialog.no_results")}`;
    }
    scrollToBottom();

    history.push({ role: "user", content: data.transcript }, { role: "assistant", content: historyReply });
    if (history.length > HISTORY_LIMIT) {
      history.splice(0, history.length - HISTORY_LIMIT);
    }
  }

  function pushEntry(entry: ChatEntry) {
    entries.value.push(entry);
    scrollToBottom();
  }

  function scrollToBottom() {
    nextTick(() => {
      if (scrollArea.value) {
        scrollArea.value.scrollTop = scrollArea.value.scrollHeight;
      }
    });
  }

  // ---------------------------------------------------------------------------
  // Action execution (client-side, via the regular entities API)
  // ---------------------------------------------------------------------------

  /**
   * Resolve a location path like "Kitchen/Shelf A" against the location tree,
   * matching segment names case-insensitively.
   */
  async function findLocationByPath(path: string): Promise<TreeItem | null> {
    const segments = path
      .split("/")
      .map(s => s.trim())
      .filter(Boolean);
    if (segments.length === 0) return null;

    if (locationStore.tree === null) {
      await locationStore.refreshTree();
    }

    let nodes = locationStore.tree ?? [];
    let current: TreeItem | null = null;
    for (const segment of segments) {
      current = nodes.find(n => n.name.toLowerCase() === segment.toLowerCase()) ?? null;
      if (!current) return null;
      nodes = current.children ?? [];
    }
    return current;
  }

  async function confirmAction(action: ActionState) {
    acting.value = true;
    try {
      if (action.type === "create_location") {
        await createLocation(action);
      } else if (action.type === "create_item") {
        await createItem(action);
      }
    } finally {
      acting.value = false;
    }
  }

  async function createLocation(action: ActionState) {
    await entityTypeStore.ensureFetched();
    const locationType = entityTypeStore.locationTypes[0];
    if (!locationType) {
      toast.error(t("assistant.dialog.create_failed"));
      return;
    }

    let parentId: string | null = null;
    if (action.parent_path) {
      const parent = await findLocationByPath(action.parent_path);
      if (parent) {
        parentId = parent.id;
      } else {
        action.note = t("assistant.dialog.location_not_found");
      }
    }

    const { error } = await api.items.createLocation({
      name: action.name,
      description: "",
      parentId,
      entityTypeId: locationType.id,
      quantity: 1,
      tagIds: [],
      productionDate: "",
      expiryDate: "",
    });

    if (error) {
      toast.error(t("assistant.dialog.create_failed"));
      return;
    }

    action.status = "done";
    toast.success(t("assistant.dialog.create_success"));
    await locationStore.refreshChildren();
    await locationStore.refreshTree();
  }

  async function createItem(action: ActionState) {
    await entityTypeStore.ensureFetched();
    const itemType = entityTypeStore.itemTypes[0];
    if (!itemType) {
      toast.error(t("assistant.dialog.create_failed"));
      return;
    }

    let parentId: string | null = null;
    if (action.location_path) {
      const location = await findLocationByPath(action.location_path);
      if (location) {
        parentId = location.id;
      } else {
        action.note = t("assistant.dialog.location_not_found");
      }
    }

    const { error } = await api.items.create({
      name: action.name,
      description: action.description || "",
      parentId,
      quantity: action.quantity && action.quantity >= 1 ? action.quantity : 1,
      tagIds: [],
      entityTypeId: itemType.id,
      productionDate: toDateOnlyString(action.production_date),
      shelfLifeDays: action.shelf_life_days > 0 ? action.shelf_life_days : null,
      expiryDate: toDateOnlyString(action.expiry_date),
    });

    if (error) {
      toast.error(t("assistant.dialog.create_failed"));
      return;
    }

    action.status = "done";
    toast.success(t("assistant.dialog.create_success"));
  }

  async function runQueryAction(action: ActionState): Promise<QueryResult[]> {
    const keyword = (action.keyword || "").trim();
    if (!keyword) return [];

    if (action.type === "query_item") {
      const { data, error } = await api.items.getAll({ q: keyword, pageSize: 5 });
      if (error || !data) return [];
      return (data.items ?? []).map(i => ({
        id: i.id,
        kind: "item" as const,
        name: i.name,
        location: i.parent?.name ?? "",
      }));
    }

    // query_location: match against the already-loaded location store
    await locationStore.ensureLocationsFetched();
    const kw = keyword.toLowerCase();
    return locationStore.allLocations
      .filter(l => l.name.toLowerCase().includes(kw))
      .slice(0, 5)
      .map(l => ({
        id: l.id,
        kind: "location" as const,
        name: l.name,
        location: l.parent?.name ?? "",
      }));
  }

  // ---------------------------------------------------------------------------
  // Dialog lifecycle: stop recording/speaking when the dialog closes
  // ---------------------------------------------------------------------------

  watch(activeDialog, active => {
    if (active !== DialogID.Assistant) {
      if (recording.value) stopRecording(true);
      stopSpeaking();
    }
  });

  onUnmounted(() => {
    if (recording.value) stopRecording(true);
    stopSpeaking();
  });
</script>
