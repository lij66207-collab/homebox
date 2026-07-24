<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import MdiHome from "~icons/mdi/home";
  import MdiMagnify from "~icons/mdi/magnify";
  import MdiFileTree from "~icons/mdi/file-tree";
  import MdiAccount from "~icons/mdi/account";
  import MdiPlus from "~icons/mdi/plus";
  import { useDialog } from "~/components/ui/dialog-provider";
  import { DialogID } from "~/components/ui/dialog-provider/utils";

  const { t } = useI18n();
  const { openDialog } = useDialog();
  const route = useRoute();

  type Tab = {
    icon: Component;
    name: ComputedRef<string>;
    active: ComputedRef<boolean>;
    to: string;
  };

  // Order matters: the create button is rendered in the middle of the bar (index 2)
  const tabs: Tab[] = [
    {
      icon: MdiHome,
      name: computed(() => t("menu.home")),
      active: computed(() => route.path === "/home"),
      to: "/home",
    },
    {
      icon: MdiMagnify,
      name: computed(() => t("global.search")),
      active: computed(() => route.path.startsWith("/item")),
      to: "/items",
    },
    {
      icon: MdiFileTree,
      name: computed(() => t("menu.locations")),
      active: computed(() => route.path.startsWith("/location")),
      to: "/locations",
    },
    {
      icon: MdiAccount,
      name: computed(() => t("menu.profile")),
      active: computed(() => route.path.startsWith("/profile")),
      to: "/profile",
    },
  ];

  function openCreateDialog() {
    openDialog(DialogID.CreateEntity, { params: { baseType: "item" } });
  }
</script>

<template>
  <nav
    class="fixed inset-x-0 bottom-0 z-40 w-full border-t bg-background/90 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
    :aria-label="$t('global.navigate')"
  >
    <div class="grid grid-cols-5">
      <template v-for="(tab, index) in tabs" :key="tab.to">
        <div v-if="index === 2" class="flex min-h-14 items-start justify-center">
          <button
            type="button"
            class="-mt-6 flex size-14 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-fab transition-transform duration-150 ease-out-expo active:scale-95"
            :aria-label="$t('global.create')"
            @click="openCreateDialog"
          >
            <MdiPlus class="size-7" />
          </button>
        </div>
        <NuxtLink
          :to="tab.to"
          class="flex min-h-14 flex-col items-center justify-center gap-0.5"
          :class="tab.active.value ? 'text-primary' : 'text-muted-foreground'"
        >
          <span
            class="flex h-7 w-14 items-center justify-center rounded-full transition-colors duration-200 ease-out-expo"
            :class="tab.active.value ? 'bg-primary/15 dark:bg-primary/25' : 'bg-transparent'"
          >
            <component :is="tab.icon" class="size-6" />
          </span>
          <span class="text-[10px] leading-none" :class="tab.active.value ? 'font-medium' : ''">{{
            tab.name.value
          }}</span>
        </NuxtLink>
      </template>
    </div>
  </nav>
</template>
