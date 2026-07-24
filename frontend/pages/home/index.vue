<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import { statCardData } from "./statistics";
  import { itemsTable } from "./table";
  import { useTagStore } from "~/stores/tags";
  import { useLocationStore } from "~~/stores/locations";
  import BaseContainer from "@/components/Base/Container.vue";
  import BaseCard from "@/components/Base/Card.vue";
  import Subtitle from "~/components/global/Subtitle.vue";
  import StatCard from "~/components/global/StatCard/StatCard.vue";
  import EmptyState from "~/components/global/EmptyState.vue";
  import ItemCard from "~/components/Item/Card.vue";
  import LocationCard from "~/components/Location/Card.vue";
  import TagChip from "~/components/Tag/Chip.vue";
  import Table from "~/components/Item/View/Table.vue";
  import { Skeleton } from "@/components/ui/skeleton";
  import { useDialog } from "@/components/ui/dialog-provider";
  import { DialogID } from "~/components/ui/dialog-provider/utils";

  const { t, locale } = useI18n();

  const { openDialog } = useDialog();

  const auth = useAuthContext();

  const greeting = computed(() => {
    const hour = new Date().getHours();
    if (hour < 12) return t("home.greeting_morning");
    if (hour < 18) return t("home.greeting_afternoon");
    return t("home.greeting_evening");
  });

  const today = computed(() => new Intl.DateTimeFormat(locale.value, { dateStyle: "full" }).format(new Date()));

  definePageMeta({
    middleware: ["auth"],
  });
  useHead({
    title: "LJJ Organizer | " + t("menu.home"),
  });

  const api = useUserApi();
  const breakpoints = useBreakpoints();

  const locationStore = useLocationStore();
  const locations = computed(() => locationStore.parentLocations);

  const tagsStore = useTagStore();
  const tags = computed(() => tagsStore.tags);

  const itemTable = itemsTable(api);
  const { stats, pending: statsPending } = statCardData(api);

  // .stagger-item uses fill-mode "both", which would keep overriding transforms
  // (e.g. ItemCard's hover:scale) after the entrance animation has finished
  function removeStaggerClass(event: AnimationEvent) {
    (event.currentTarget as HTMLElement).classList.remove("stagger-item");
  }
</script>

<template>
  <div>
    <BaseContainer class="flex flex-col gap-4">
      <header class="animate-fade-in-up px-1 pt-2">
        <h1 class="text-2xl font-bold tracking-tight sm:text-3xl">
          {{ greeting }}{{ auth.user?.name ? `, ${auth.user.name}` : "" }}
        </h1>
        <p class="mt-1 text-sm text-muted-foreground">{{ today }}</p>
      </header>

      <section>
        <Subtitle> {{ $t("home.quick_statistics") }} </Subtitle>
        <div class="grid grid-cols-2 gap-2 md:grid-cols-4 md:gap-6">
          <template v-if="statsPending">
            <Skeleton v-for="i in 4" :key="i" class="h-24 rounded-xl" />
          </template>
          <template v-else>
            <StatCard
              v-for="(stat, i) in stats"
              :key="i"
              :title="stat.label"
              :value="stat.value"
              :type="stat.type"
              :icon="stat.icon"
              class="stagger-item"
              :style="{ '--i': i }"
              @animationend="removeStaggerClass"
            />
          </template>
        </div>
      </section>

      <section>
        <Subtitle> {{ $t("home.recently_added") }} </Subtitle>

        <EmptyState
          v-if="itemTable.items.length === 0"
          icon="package-variant"
          :title="$t('components.global.empty_state.items_empty.title')"
          :description="$t('components.global.empty_state.items_empty.description')"
          :action-label="$t('components.global.empty_state.items_empty.action')"
          @action="openDialog(DialogID.CreateEntity, { params: { baseType: 'item' } })"
        />
        <BaseCard v-else-if="breakpoints.lg">
          <Table :items="itemTable.items" />
        </BaseCard>
        <div v-else class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <ItemCard
            v-for="(item, index) in itemTable.items"
            :key="item.id"
            :item="item"
            class="stagger-item"
            :style="{ '--i': Math.min(index, 10) }"
            @animationend="removeStaggerClass"
          />
        </div>
      </section>

      <section>
        <Subtitle> {{ $t("home.storage_locations") }} </Subtitle>
        <p v-if="locations.length === 0" class="ml-2 text-sm">{{ $t("locations.no_results") }}</p>
        <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3">
          <LocationCard v-for="location in locations" :key="location.id" :location="location" />
        </div>
      </section>

      <section>
        <Subtitle> {{ $t("home.tags") }} </Subtitle>
        <p v-if="tags.length === 0" class="ml-2 text-sm">{{ $t("tags.no_results") }}</p>
        <div v-else class="flex flex-wrap gap-4">
          <TagChip v-for="tag in tags" :key="tag.id" size="lg" :tag="tag" class="shadow-md" />
        </div>
      </section>
    </BaseContainer>
  </div>
</template>
