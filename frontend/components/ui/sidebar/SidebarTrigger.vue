<script setup lang="ts">
  import type { HTMLAttributes } from "vue";
  import { useI18n } from "vue-i18n";
  import { useSidebar } from "./utils";
  import { Button } from "@/components/ui/button";
  import { cn } from "@/lib/utils";
  import MdiMenu from "~icons/mdi/menu";

  const props = defineProps<{
    class?: HTMLAttributes["class"];
    variant?: "ghost" | "default";
  }>();

  const { toggleSidebar } = useSidebar();
  const { t } = useI18n();
</script>

<template>
  <Button data-sidebar="trigger" :variant="props.variant ?? 'ghost'" size="icon" :class="cn('size-9 [&_svg]:size-6', props.class)" @click="toggleSidebar">
    <MdiMenu class="text-primary-foreground" />
    <span class="sr-only">{{ t("components.ui.sidebar.toggle") }}</span>
  </Button>
</template>
