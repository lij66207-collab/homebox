<template>
  <div id="app">
    <!--
    Confirmation Modal is a singleton used by all components so we render
    it here to ensure it's always available. Possibly could move this further
    up the tree
    -->
    <ModalConfirm />
    <OutdatedModal v-if="status" :status="status" />
    <EntityCreateModal />
    <WipeInventoryDialog />
    <TagCreateModal />
    <ItemBarcodeModal />
    <AppQuickMenuModal :actions="quickMenuActions" />
    <AppScannerModal />
    <CollectionCreateModal />
    <CollectionJoinModal />
    <CollectionInviteCreateModal />
    <SidebarProvider :default-open="sidebarState">
      <Sidebar collapsible="icon">
        <SidebarHeader class="gap-3 px-3 pt-4">
          <NuxtLink to="/home" class="flex items-center gap-2.5 px-1 group-data-[collapsible=icon]:justify-center">
            <div
              class="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary/10 p-1.5 text-primary dark:bg-primary/20"
            >
              <AppLogo />
            </div>
            <span class="text-lg font-bold tracking-tight group-data-[collapsible=icon]:hidden">LJJ Organizer</span>
          </NuxtLink>

          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <SidebarMenuButton
                class="flex h-10 justify-center rounded-xl bg-primary text-primary-foreground shadow-sm transition-all duration-150 hover:bg-primary/90 active:scale-[0.98] active:bg-primary/90 active:text-primary-foreground group-data-[collapsible=icon]:justify-start"
                :tooltip="$t('global.create')"
                :hotkey="$t('global.shortcut', { keys: 'Ctrl+`' })"
              >
                <MdiPlus />
                <span>
                  {{ $t("global.create") }}
                </span>
              </SidebarMenuButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent class="z-40 min-w-[var(--reka-dropdown-menu-trigger-width)]">
              <DropdownMenuItem
                v-for="btn in dropdown"
                :key="btn.id"
                class="group cursor-pointer text-lg"
                @click="
                  () => {
                    if (btn.dialogId === DialogID.CreateEntity) {
                      if (btn.id == 0)
                        // create item
                        openDialog(btn.dialogId, { params: { baseType: 'item' } });
                      else if (btn.id == 1)
                        // create location
                        openDialog(btn.dialogId, { params: { baseType: 'location' } });
                    } else {
                      openDialog(btn.dialogId as NoParamDialogIDs);
                    }
                  }
                "
              >
                {{ btn.name.value }}
                <Shortcut
                  v-if="btn.shortcut"
                  class="invisible ml-auto group-hover:visible"
                  :keys="btn.shortcut.replace('Shift', '⇧').split('+')"
                />
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <CollectionSelector />
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup v-for="group in navGroups" :key="group.id">
            <SidebarGroupLabel
              class="text-[11px] font-medium uppercase tracking-wider text-muted-foreground group-data-[collapsible=icon]:hidden"
            >
              {{ $t(group.label) }}
            </SidebarGroupLabel>
            <SidebarMenu>
              <template v-for="n in group.items" :key="n.id">
                <SidebarMenuItem v-if="!n.collapsible" :key="n.id">
                  <SidebarMenuLink
                    :href="n.to"
                    class="rounded-full transition-colors duration-200"
                    :class="{
                      'bg-primary/10 font-medium text-primary dark:bg-primary/20': n.active?.value,
                      'text-nowrap': typeof locale === 'string' && locale.startsWith('zh-'),
                    }"
                    :tooltip="n.name.value"
                  >
                    <component :is="n.icon" />
                    <span>{{ n.name.value }}</span>
                  </SidebarMenuLink>
                </SidebarMenuItem>

                <Collapsible v-else default-open class="group/collapsible">
                  <SidebarMenuItem>
                    <SidebarMenuItem class="flex gap-1">
                      <SidebarMenuLink
                        :href="n.to"
                        class="rounded-full transition-colors duration-200"
                        :class="{
                          'bg-primary/10 font-medium text-primary dark:bg-primary/20': n.active?.value,
                          'text-nowrap': typeof locale === 'string' && locale.startsWith('zh-'),
                        }"
                        :tooltip="n.name.value"
                      >
                        <component :is="n.icon" />
                        <span>{{ n.name.value }}</span>
                      </SidebarMenuLink>
                      <CollapsibleTrigger as-child>
                        <SidebarMenuButton class="flex size-12 items-center justify-center">
                          <MdiChevronRight
                            class="transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90"
                          />
                        </SidebarMenuButton>
                      </CollapsibleTrigger>
                    </SidebarMenuItem>
                    <CollapsibleContent>
                      <SidebarMenuSub>
                        <SidebarMenuSubItem v-for="c in n.collapsible" :key="c.id">
                          <SidebarMenuLink
                            :href="c.to"
                            class="rounded-full transition-colors duration-200"
                            :class="{
                              'bg-primary/10 font-medium text-primary dark:bg-primary/20': c.active?.value,
                              'text-nowrap': typeof locale === 'string' && locale.startsWith('zh-'),
                              'h-min py-0': true,
                            }"
                            :tooltip="c.name.value"
                          >
                            <span>{{ c.name.value }}</span>
                          </SidebarMenuLink>
                        </SidebarMenuSubItem>
                      </SidebarMenuSub>
                    </CollapsibleContent>
                  </SidebarMenuItem>
                </Collapsible>
              </template>

              <!-- makes scanner accessible easily if using legacy header -->
              <SidebarMenuItem v-if="preferences.displayLegacyHeader && group.id === 'system'">
                <SidebarMenuButton
                  :class="{
                    'text-nowrap': typeof locale === 'string' && locale.startsWith('zh-'),
                  }"
                  :tooltip="$t('menu.scanner')"
                  @click.prevent="openDialog(DialogID.Scanner)"
                >
                  <MdiQrcodeScan />
                  <span>{{ $t("menu.scanner") }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter>
          <div class="flex items-center gap-2 rounded-xl p-1 group-data-[collapsible=icon]:justify-center">
            <div
              class="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/15 text-sm font-semibold text-primary dark:bg-primary/25"
            >
              {{ username.charAt(0).toUpperCase() }}
            </div>
            <p class="min-w-0 flex-1 truncate text-sm font-medium group-data-[collapsible=icon]:hidden">
              {{ username }}
            </p>
            <SidebarMenuButton
              class="size-8 shrink-0 grow-0"
              :tooltip="$t('global.sign_out')"
              data-testid="logout-button"
              @click="logout"
            >
              <MdiLogout />
            </SidebarMenuButton>
          </div>
          <p
            v-if="status"
            class="px-2 pb-1 text-center text-xs text-muted-foreground group-data-[collapsible=icon]:hidden"
          >
            <span
              v-html="
                DOMPurify.sanitize(
                  $t('global.footer.version_link', {
                    version: status.build.version.replace(/^v/, ''),
                    build: status.build.commit,
                  })
                )
              "
            />
            ~
            <span v-html="DOMPurify.sanitize($t('global.footer.api_link'))" />
          </p>
        </SidebarFooter>

        <SidebarRail />
      </Sidebar>
      <SidebarInset class="min-h-dvh max-w-full overflow-hidden bg-background-accent">
        <div class="relative flex min-h-dvh flex-col pb-20 md:pb-0">
          <div v-if="preferences.displayLegacyHeader">
            <AppHeaderDecor class="-mt-10 hidden lg:block" />
            <SidebarTrigger class="absolute left-2 top-2 hidden lg:flex" variant="default" />
          </div>
          <!-- IMPORTANT: if you change the height of this div, alter the top value in the item edit page-->
          <div
            class="sticky top-0 z-20 flex h-[var(--header-height)] translate-y-[-0.5px] items-center gap-2 border-b bg-background/80 px-3 backdrop-blur"
            :class="{
              'lg:hidden': preferences.displayLegacyHeader,
            }"
          >
            <SidebarTrigger variant="default" />
            <h1 class="truncate text-lg font-semibold tracking-tight">{{ pageTitle }}</h1>
            <div class="grow" />
            <div class="hidden items-center gap-2 sm:flex">
              <Input
                v-model:model-value="search"
                class="h-9 w-52 rounded-full lg:w-64"
                :placeholder="$t('global.search')"
                type="search"
                @keyup.enter="triggerSearch"
              />
              <Button size="icon" class="rounded-full" :aria-label="$t('global.search')" @click="triggerSearch">
                <MdiMagnify />
              </Button>
            </div>
            <Button
              size="icon"
              variant="ghost"
              class="rounded-full"
              :aria-label="$t('menu.scanner')"
              @click="openScanner"
            >
              <MdiQrcodeScan />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              class="rounded-full"
              :aria-label="$t('global.toggle_theme')"
              @click="toggleTheme"
            >
              <MdiMoonWaningCrescent v-if="!isDark" class="transition-transform duration-300 hover:rotate-12" />
              <MdiWhiteBalanceSunny v-else class="transition-transform duration-300 hover:rotate-45" />
            </Button>
          </div>

          <div v-if="breadcrumbs.length > 0" class="hidden px-4 py-2 text-sm md:block">
            <Breadcrumb>
              <BreadcrumbList>
                <template v-for="(crumb, index) in breadcrumbs" :key="index">
                  <BreadcrumbItem>
                    <BreadcrumbLink v-if="crumb.to" as-child>
                      <NuxtLink :to="crumb.to">{{ crumb.label }}</NuxtLink>
                    </BreadcrumbLink>
                    <BreadcrumbPage v-else>{{ crumb.label }}</BreadcrumbPage>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator v-if="index < breadcrumbs.length - 1" />
                </template>
              </BreadcrumbList>
            </Breadcrumb>
          </div>

          <slot />
        </div>
      </SidebarInset>
    </SidebarProvider>
    <AppMobileTabBar />
  </div>
</template>

<script lang="ts" setup>
  import { useI18n } from "vue-i18n";
  import DOMPurify from "dompurify";
  import { useTagStore } from "~/stores/tags";
  import { useLocationStore } from "~~/stores/locations";
  import { useEntityTypeStore } from "~~/stores/entityTypes";

  import MdiHome from "~icons/mdi/home";
  import MdiFileTree from "~icons/mdi/file-tree";
  import MdiTagMultiple from "~icons/mdi/tag-multiple";
  import MdiMagnify from "~icons/mdi/magnify";
  import MdiQrcodeScan from "~icons/mdi/qrcode-scan";
  import MdiAccount from "~icons/mdi/account";
  import MdiCog from "~icons/mdi/cog";
  import MdiWrench from "~icons/mdi/wrench";
  import MdiPlus from "~icons/mdi/plus";
  import MdiLogout from "~icons/mdi/logout";
  import MdiMoonWaningCrescent from "~icons/mdi/moon-waning-crescent";
  import MdiWhiteBalanceSunny from "~icons/mdi/white-balance-sunny";
  import MdiShieldAccount from "~icons/mdi/shield-account";
  import MdiFileDocumentMultiple from "~icons/mdi/file-document-multiple";
  import MdiChevronRight from "~icons/mdi/chevron-right";

  import {
    Sidebar,
    SidebarContent,
    SidebarFooter,
    SidebarGroup,
    SidebarGroupLabel,
    SidebarHeader,
    SidebarInset,
    SidebarMenu,
    SidebarMenuSub,
    SidebarMenuSubItem,
    SidebarMenuButton,
    SidebarMenuItem,
    SidebarMenuLink,
    SidebarProvider,
    SidebarRail,
    SidebarTrigger,
  } from "@/components/ui/sidebar";
  import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
  } from "@/components/ui/dropdown-menu";
  import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
  import {
    Breadcrumb,
    BreadcrumbItem,
    BreadcrumbLink,
    BreadcrumbList,
    BreadcrumbPage,
    BreadcrumbSeparator,
  } from "@/components/ui/breadcrumb";
  import { Shortcut } from "~/components/ui/shortcut";
  import { useDialog } from "~/components/ui/dialog-provider";
  import { Input } from "~/components/ui/input";
  import { Button } from "~/components/ui/button";
  import { toast } from "@/components/ui/sonner";
  import { DialogID, type NoParamDialogIDs } from "~/components/ui/dialog-provider/utils";
  import ModalConfirm from "~/components/ModalConfirm.vue";
  import OutdatedModal from "~/components/App/OutdatedModal.vue";
  import EntityCreateModal from "~/components/Entity/CreateModal.vue";
  import WipeInventoryDialog from "~/components/WipeInventoryDialog.vue";
  import TagCreateModal from "~/components/Tag/CreateModal.vue";
  import ItemBarcodeModal from "~/components/Item/BarcodeModal.vue";
  import AppQuickMenuModal from "~/components/App/QuickMenuModal.vue";
  import AppMobileTabBar from "~/components/App/MobileTabBar.vue";
  import AppScannerModal from "~/components/App/ScannerModal.vue";
  import AppLogo from "~/components/App/Logo.vue";
  import AppHeaderDecor from "~/components/App/HeaderDecor.vue";
  import CollectionSelector from "~/components/Collection/Selector.vue";
  import CollectionCreateModal from "~/components/Collection/CreateModal.vue";
  import CollectionJoinModal from "~/components/Collection/JoinModal.vue";
  import CollectionInviteCreateModal from "~/components/Collection/InviteCreateModal.vue";

  const { t, locale } = useI18n();
  const username = computed(() => authCtx.user?.name || "User");

  const { isDark, toggleTheme } = useTheme();

  const { openDialog } = useDialog();

  const authCtx = useAuthContext();

  const preferences = useViewPreferences();

  // get sidebar state from cookies
  const sidebarState = useCookie("sidebar:state", {
    readonly: true,
    decode: value => value !== "false",
  });

  const pubApi = usePublicApi();
  const { data: status } = useAsyncData(async () => {
    const { data } = await pubApi.status();

    return data;
  });

  const search = ref("");

  const triggerSearch = () => {
    if (search.value) {
      navigateTo(`/items?q=${encodeURIComponent(search.value)}`);
      search.value = "";
      // remove focus from input
      if (document.activeElement && "blur" in document.activeElement) {
        (document.activeElement as HTMLElement).blur();
      }
    }
  };

  const openScanner = () => {
    // request permission
    if (navigator.mediaDevices) {
      navigator.mediaDevices
        .getUserMedia({ video: true })
        .then(() => {
          openDialog(DialogID.Scanner);
        })
        .catch(err => {
          console.error(err);
          toast.error(t("scanner.permission_denied"));
        });
    } else {
      toast.error(t("scanner.unsupported"));
    }
  };

  // Preload currency format
  useFormatCurrency();

  type DropdownItem = {
    id: number;
    name: ComputedRef<string>;
    shortcut: string;
    dialogId: DialogID;
  };

  const dropdown: DropdownItem[] = [
    {
      id: 0,
      name: computed(() => t("menu.create_item")),
      shortcut: "Shift+1",
      dialogId: DialogID.CreateEntity,
    },
    {
      id: 1,
      name: computed(() => t("menu.create_location")),
      shortcut: "Shift+2",
      dialogId: DialogID.CreateEntity,
    },
    {
      id: 2,
      name: computed(() => t("menu.create_tag")),
      shortcut: "Shift+3",
      dialogId: DialogID.CreateTag,
    },
  ];

  const route = useRoute();
  const router = useRouter();

  type NavItem = {
    icon: Component;
    active: ComputedRef<boolean>;
    id: number;
    name: ComputedRef<string>;
    to: string;
    collapsible?: {
      active: ComputedRef<boolean>;
      id: number;
      name: ComputedRef<string>;
      to: string;
    }[];
  };

  const navGroups = computed<{ id: string; label: string; items: NavItem[] }[]>(() => {
    const groups: { id: string; label: string; items: NavItem[] }[] = [
      {
        id: "overview",
        label: "menu.group_overview",
        items: [
          {
            icon: MdiHome,
            active: computed(() => route.path === "/home"),
            id: 0,
            name: computed(() => t("menu.home")),
            to: "/home",
          },
          {
            icon: MdiMagnify,
            id: 3,
            active: computed(() => route.path === "/items"),
            name: computed(() => t("menu.search")),
            to: "/items",
          },
        ],
      },
      {
        id: "manage",
        label: "menu.group_manage",
        items: [
          {
            icon: MdiFileTree,
            id: 1,
            active: computed(() => route.path === "/locations"),
            name: computed(() => t("menu.locations")),
            to: "/locations",
          },
          {
            icon: MdiTagMultiple,
            id: 2,
            active: computed(() => route.path === "/tags"),
            name: computed(() => t("global.tags")),
            to: "/tags",
          },
          {
            icon: MdiFileDocumentMultiple,
            id: 4,
            active: computed(() => route.path === "/templates"),
            name: computed(() => t("menu.templates")),
            to: "/templates",
          },
          {
            icon: MdiWrench,
            id: 5,
            active: computed(() => route.path === "/maintenance"),
            name: computed(() => t("menu.maintenance")),
            to: "/maintenance",
          },
        ],
      },
      {
        id: "system",
        label: "menu.group_system",
        items: [
          {
            icon: MdiCog,
            id: 7,
            active: computed(() => route.path.includes("/collection")),
            name: computed(() => t("menu.collection")),
            to: "/collection/members",
            collapsible: [
              {
                id: 61,
                active: computed(() => route.path === "/collection/members"),
                name: computed(() => t("collection.tabs.members")),
                to: "/collection/members",
              },
              {
                id: 62,
                active: computed(() => route.path === "/collection/invites"),
                name: computed(() => t("collection.tabs.invites")),
                to: "/collection/invites",
              },
              {
                id: 63,
                active: computed(() => route.path === "/collection/notifiers"),
                name: computed(() => t("collection.tabs.notifiers")),
                to: "/collection/notifiers",
              },
              {
                id: 64,
                active: computed(() => route.path === "/collection/settings"),
                name: computed(() => t("collection.tabs.settings")),
                to: "/collection/settings",
              },
              {
                id: 65,
                active: computed(() => route.path === "/collection/entity-types"),
                name: computed(() => t("collection.tabs.entity_types")),
                to: "/collection/entity-types",
              },
              {
                id: 66,
                active: computed(() => route.path === "/collection/tools"),
                name: computed(() => t("collection.tabs.tools")),
                to: "/collection/tools",
              },
            ],
          },
          {
            icon: MdiAccount,
            id: 6,
            active: computed(() => route.path === "/profile"),
            name: computed(() => t("menu.profile")),
            to: "/profile",
          },
        ],
      },
    ];

    if (authCtx.user?.isSuperuser) {
      groups
        .find(g => g.id === "system")!
        .items.push({
          icon: MdiShieldAccount,
          id: 8,
          active: computed(() => route.path.startsWith("/admin")),
          name: computed(() => t("menu.admin")),
          to: "/admin",
        });
    }

    return groups;
  });

  const pageTitle = computed(() => {
    const path = route.path;
    const segments = path.split("/").filter(Boolean);
    const labelKey = staticCrumbLabels[segments[0] ?? ""];
    if (labelKey) return t(labelKey);
    if (segments[0] === "item") return t("global.item");
    if (segments[0] === "location") return t("menu.locations");
    if (segments[0] === "tag") return t("global.tags");
    return "LJJ Organizer";
  });

  const quickMenuActions = reactive([
    ...dropdown.map(v => ({
      text: computed(() => v.name.value),
      dialogId: v.dialogId,
      shortcut: v.shortcut.split("+")[1] as string,
      id: v.id,
      type: "create" as const,
    })),
    ...navGroups.value
      .flatMap(g => g.items)
      .map(v => ({
        text: computed(() => v.name.value),
        href: v.to,
        type: "navigate" as const,
      })),
  ]);

  const tagStore = useTagStore();
  tagStore.ensureAllTagsFetched();

  const locationStore = useLocationStore();
  locationStore.ensureLocationsFetched();

  const entityTypeStore = useEntityTypeStore();
  entityTypeStore.ensureFetched();

  type BreadcrumbEntry = {
    label: string;
    to?: string;
  };

  const staticCrumbLabels: Record<string, string> = {
    home: "menu.home",
    items: "menu.search",
    locations: "menu.locations",
    tags: "global.tags",
    profile: "menu.profile",
    maintenance: "menu.maintenance",
    templates: "menu.templates",
    collection: "menu.collection",
    admin: "menu.admin",
  };

  const breadcrumbs = computed<BreadcrumbEntry[]>(() => {
    const path = route.path;
    if (path === "/home" || path === "/") return [];

    const segments = path.split("/").filter(Boolean);
    if (segments.length === 0) return [];

    const [first, second] = segments;

    // Detail pages: resolve the entity name from the stores when possible
    if (segments.length === 2 && second) {
      if (first === "item") {
        return [{ label: t("menu.search"), to: "/items" }, { label: t("global.item") }];
      }
      if (first === "location") {
        const location = locationStore.allLocations.find(l => l.id === second);
        const crumbs: BreadcrumbEntry[] = [{ label: t("menu.locations"), to: "/locations" }];
        if (location) crumbs.push({ label: location.name });
        return crumbs;
      }
      if (first === "tag") {
        const tag = tagStore.tags.find(entry => entry.id === second);
        const crumbs: BreadcrumbEntry[] = [{ label: t("global.tags"), to: "/tags" }];
        if (tag) crumbs.push({ label: tag.name });
        return crumbs;
      }
    }

    // Static pages: map known segments to their menu labels
    const crumbs: BreadcrumbEntry[] = [];
    segments.forEach((segment, index) => {
      const labelKey = staticCrumbLabels[segment];
      if (!labelKey) return;
      const isLast = index === segments.length - 1;
      crumbs.push({
        label: t(labelKey),
        to: isLast ? undefined : `/${segments.slice(0, index + 1).join("/")}`,
      });
    });
    return crumbs;
  });

  onMounted(() => {
    locationStore.refreshParents();
    locationStore.refreshTree();

    // Auto-open JoinModal when invitation token is in URL
    const token = route.query.token;
    if (typeof token === "string" && token.length > 0) {
      // Remove token from browser URL
      const url = new URL(window.location.href);
      url.searchParams.delete("token");
      window.history.replaceState(history.state, "", url.toString());

      // Sync router's state to clear route.query.token
      const { token: _, ...cleanQuery } = route.query;
      router.replace({ query: cleanQuery });

      openDialog(DialogID.JoinCollection, {
        params: { inviteCode: token },
      });
    }
  });

  onServerEvent(ServerEvent.TagMutation, () => {
    tagStore.refresh();
  });

  onServerEvent(ServerEvent.EntityMutation, () => {
    locationStore.refreshChildren();
    locationStore.refreshParents();
    locationStore.refreshTree();
  });

  const api = useUserApi();

  async function logout() {
    await authCtx.logout(api);
    navigateTo("/");
  }
</script>
