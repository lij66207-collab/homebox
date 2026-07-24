<script setup lang="ts">
  import MdiPackageVariant from "~icons/mdi/package-variant";
  import MdiMagnify from "~icons/mdi/magnify";
  import MdiMapMarkerOff from "~icons/mdi/map-marker-off";
  import MdiTagOff from "~icons/mdi/tag-off";
  import { Button } from "@/components/ui/button";

  // Allowlist of supported icons to keep the bundle small
  const icons = {
    "package-variant": MdiPackageVariant,
    magnify: MdiMagnify,
    "map-marker-off": MdiMapMarkerOff,
    "tag-off": MdiTagOff,
  } as const;

  const props = withDefaults(
    defineProps<{
      icon: keyof typeof icons;
      title: string;
      description?: string;
      actionLabel?: string;
      to?: string;
      compact?: boolean;
    }>(),
    {
      description: undefined,
      actionLabel: undefined,
      to: undefined,
      compact: false,
    }
  );

  const emit = defineEmits<{
    (e: "action"): void;
  }>();

  const iconComponent = computed(() => icons[props.icon]);
</script>

<template>
  <div
    class="flex animate-fade-in-up flex-col items-center justify-center text-center"
    :class="compact ? 'gap-1 py-4' : 'gap-2 py-12'"
  >
    <div
      class="flex items-center justify-center rounded-full bg-muted text-muted-foreground"
      :class="compact ? 'size-12' : 'size-20'"
    >
      <component :is="iconComponent" :class="compact ? 'size-6' : 'size-10'" />
    </div>
    <h3 class="text-base font-semibold" :class="compact ? '' : 'mt-2'">{{ title }}</h3>
    <p v-if="description" class="max-w-sm text-sm text-muted-foreground">{{ description }}</p>
    <template v-if="actionLabel && !compact">
      <Button v-if="to" as-child class="mt-2 min-h-11">
        <NuxtLink :to="to">{{ actionLabel }}</NuxtLink>
      </Button>
      <Button v-else class="mt-2 min-h-11" @click="emit('action')">{{ actionLabel }}</Button>
    </template>
  </div>
</template>
