<script setup lang="ts">
  import type { TreeItem } from "~~/lib/api/types/data-contracts";
  import LocationTreeNode from "./Node.vue";
  import { Button } from "~~/components/ui/button";
  import { useDialog } from "~/components/ui/dialog-provider";
  import { DialogID } from "~/components/ui/dialog-provider/utils";

  type Props = {
    locs: TreeItem[];
    treeId: string;
    showItems?: boolean;
  };

  const { openDialog } = useDialog();

  const props = defineProps<Props>();

  const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

  const sortedLocs = computed(() => {
    const list = props.locs ?? [];
    return [...list].sort((a, b) => collator.compare(a.name, b.name));
  });

  // .stagger-item uses fill-mode "both", which would keep overriding transforms
  // after the entrance finishes — remove it once the animation ends.
  function removeStaggerClass(event: AnimationEvent) {
    (event.currentTarget as HTMLElement).classList.remove("stagger-item");
  }
</script>

<template>
  <div>
    <div
      v-if="sortedLocs.length === 0"
      class="py-6 text-center text-sm text-muted-foreground"
      role="status"
      aria-live="polite"
    >
      <p class="mx-auto max-w-xs">
        {{ $t("components.location.tree.no_locations") }}
      </p>
      <Button
        class="mt-3"
        variant="outline"
        size="sm"
        type="button"
        :aria-label="$t('global.create')"
        @click="openDialog(DialogID.CreateEntity, { params: { baseType: 'location' } })"
      >
        {{ $t("global.create") }}
      </Button>
    </div>

    <ul role="tree" :aria-labelledby="treeId" class="space-y-1">
      <li
        v-for="(item, index) in sortedLocs"
        :key="item.id"
        role="treeitem"
        class="stagger-item"
        :style="{ '--i': Math.min(index, 10) }"
        @animationend="removeStaggerClass"
      >
        <LocationTreeNode :item="item" :tree-id="treeId" :show-items="props.showItems ?? true" />
      </li>
    </ul>
  </div>
</template>
