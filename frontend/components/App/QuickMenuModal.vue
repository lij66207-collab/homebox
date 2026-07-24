<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { useMagicKeys } from "@vueuse/core";
  import { DialogID, type NoParamDialogIDs } from "@/components/ui/dialog-provider/utils";
  import {
    Command,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
    CommandSeparator,
  } from "~/components/ui/command";
  import { Dialog, DialogContent } from "@/components/ui/dialog";
  import { Skeleton } from "@/components/ui/skeleton";
  import { Shortcut } from "~/components/ui/shortcut";
  import { useDialog, useDialogHotkey } from "~/components/ui/dialog-provider";
  import EmptyState from "~/components/global/EmptyState.vue";
  import type { EntitySummary } from "~~/lib/api/types/data-contracts";

  export type QuickMenuAction =
    | { text: string; href: string; type: "navigate" }
    | { text: string; dialogId: DialogID; shortcut: string; type: "create"; id?: number };

  const props = defineProps({
    actions: {
      type: Array as PropType<QuickMenuAction[]>,
      required: false,
      default: () => [],
    },
  });

  const { t } = useI18n();
  const { activeDialog, closeDialog, openDialog } = useDialog();

  useDialogHotkey(DialogID.QuickMenu, { code: "Backquote", ctrl: true });

  // Ctrl+K / Cmd+K toggles the quick menu from anywhere, including text inputs.
  // Registered locally because useDialogHotkey only supports the ctrl modifier.
  useMagicKeys({
    passive: false,
    onEventFired: event => {
      if (event.type === "keydown" && event.code === "KeyK" && !event.repeat && (event.ctrlKey || event.metaKey)) {
        event.preventDefault();
        if (activeDialog.value === DialogID.QuickMenu) {
          closeDialog(DialogID.QuickMenu);
        } else {
          openDialog(DialogID.QuickMenu);
        }
      }
    },
  });

  const api = useUserApi();
  const searchQuery = ref("");
  const searchResults = ref<EntitySummary[]>([]);
  const searchLoading = ref(false);
  const hasQuery = computed(() => searchQuery.value.trim().length > 0);

  let searchSeq = 0;
  watchDebounced(
    searchQuery,
    async q => {
      const seq = ++searchSeq;
      const query = q.trim();
      if (!query) {
        searchResults.value = [];
        searchLoading.value = false;
        return;
      }

      searchLoading.value = true;
      const { data, error } = await api.items.getAll({ q: query, pageSize: 8 });
      if (seq !== searchSeq) {
        // A newer search superseded this one
        return;
      }
      searchLoading.value = false;
      searchResults.value = !error && data?.items ? data.items : [];
    },
    { debounce: 250, maxWait: 1000 }
  );

  // Reset the search state whenever the quick menu is closed
  watch(activeDialog, id => {
    if (id !== DialogID.QuickMenu) {
      searchSeq++;
      searchQuery.value = "";
      searchResults.value = [];
      searchLoading.value = false;
    }
  });

  function escapeRegExp(value: string): string {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  function highlightParts(name: string): { text: string; match: boolean }[] {
    const query = searchQuery.value.trim();
    if (!query) {
      return [{ text: name, match: false }];
    }

    const escaped = escapeRegExp(query);
    const matcher = new RegExp(`^${escaped}$`, "i");
    return name
      .split(new RegExp(`(${escaped})`, "ig"))
      .filter(part => part.length > 0)
      .map(part => ({ text: part, match: matcher.test(part) }));
  }

  function selectResult(item: EntitySummary) {
    closeDialog(DialogID.QuickMenu);
    navigateTo(`/item/${item.id}`);
  }
</script>

<template>
  <Dialog :dialog-id="DialogID.QuickMenu">
    <!-- https://github.com/unovue/reka-ui/issues/1743 -->
    <DialogContent class="overflow-hidden p-0 shadow-lg" disable-portal>
      <Command
        :ignore-filter="hasQuery"
        class="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0 [&_[cmdk-group]]:px-2 [&_[cmdk-input-wrapper]_svg]:size-5 [&_[cmdk-input]]:h-12 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-3 [&_[cmdk-item]_svg]:size-5"
      >
        <CommandInput
          v-model="searchQuery"
          :placeholder="t('components.app.quick_menu.search_placeholder')"
          @keydown="
            (e: KeyboardEvent) => {
              const item = props.actions.filter(item => 'shortcut' in item).find(item => item.shortcut === e.key);
              if (item) {
                e.preventDefault();
                if (item.dialogId === DialogID.CreateEntity) {
                  if (item.id === 0) {
                    openDialog(DialogID.CreateEntity, { params: { baseType: 'item' } });
                  } else if (item.id === 1) {
                    openDialog(DialogID.CreateEntity, { params: { baseType: 'location' } });
                  }
                } else {
                  openDialog(item.dialogId as NoParamDialogIDs);
                }
              }
              // if esc is pressed, close the dialog
              if (e.key === 'Escape') {
                e.preventDefault();
                closeDialog(DialogID.QuickMenu);
              }
            }
          "
        />
        <CommandList>
          <CommandSeparator />
          <CommandEmpty>{{ t("components.quick_menu.no_results") }}</CommandEmpty>
          <template v-if="hasQuery">
            <CommandGroup :heading="t('components.app.quick_menu.search_results')">
              <div v-if="searchLoading" class="px-2 py-1.5">
                <Skeleton class="h-4 w-full" />
              </div>
              <EmptyState
                v-else-if="searchResults.length === 0"
                compact
                icon="magnify"
                :title="t('components.global.empty_state.search_no_results.title')"
                :description="t('components.global.empty_state.search_no_results.description')"
              />
              <template v-else>
                <CommandItem
                  v-for="item in searchResults"
                  :key="item.id"
                  :value="`search-result-${item.id}`"
                  @select="selectResult(item)"
                >
                  <div class="flex min-w-0 flex-col">
                    <span class="truncate">
                      <template v-for="(part, i) in highlightParts(item.name)" :key="i">
                        <mark v-if="part.match" class="rounded-sm bg-accent px-0.5">{{ part.text }}</mark>
                        <template v-else>{{ part.text }}</template>
                      </template>
                    </span>
                    <span v-if="item.parent" class="truncate text-xs text-muted-foreground">
                      {{ item.parent.name }}
                    </span>
                  </div>
                </CommandItem>
              </template>
            </CommandGroup>
            <CommandSeparator />
          </template>
          <CommandGroup :heading="t('global.create')">
            <CommandItem
              v-for="(create, i) in props.actions.filter(item => item.type === 'create')"
              :key="`$global.create_${i + 1}`"
              :value="create.text"
              @select="
                e => {
                  e.preventDefault();
                  openDialog(create.dialogId as NoParamDialogIDs);
                }
              "
            >
              {{ create.text }}
              <Shortcut v-if="'shortcut' in create" class="ml-auto" size="sm" :keys="[create.shortcut]" />
            </CommandItem>
          </CommandGroup>
          <CommandSeparator />
          <CommandGroup :heading="t('global.navigate')">
            <CommandItem
              v-for="(navigate, i) in props.actions.filter(item => item.type === 'navigate')"
              :key="navigate.text"
              :value="`global.navigate_${i + 1}`"
              @select="
                () => {
                  closeDialog(DialogID.QuickMenu);
                  navigateTo(navigate.href);
                }
              "
            >
              {{ navigate.text }}
            </CommandItem>
            <CommandItem
              value="scanner"
              @select="
                () => {
                  closeDialog(DialogID.QuickMenu);
                  openDialog(DialogID.Scanner);
                }
              "
            >
              {{ t("menu.scanner") }}
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    </DialogContent>
  </Dialog>
</template>
