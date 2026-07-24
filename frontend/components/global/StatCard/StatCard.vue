<template>
  <Card
    class="flex flex-row items-center gap-3 p-4 transition-all duration-200 ease-out-expo hover:-translate-y-0.5 hover:shadow-card-hover"
  >
    <div
      v-if="icon"
      class="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary dark:bg-primary/20"
    >
      <component :is="icon" class="size-6" />
    </div>
    <div class="min-w-0">
      <p class="truncate text-sm font-medium text-muted-foreground">{{ title }}</p>
      <p class="text-2xl font-bold tracking-tight">
        <Currency v-if="type === 'currency'" :amount="value" />
        <template v-else>{{ displayValue }}</template>
      </p>
      <p v-if="subtitle" class="truncate text-xs text-muted-foreground">{{ subtitle }}</p>
    </div>
  </Card>
</template>

<script setup lang="ts">
  import Currency from "../Currency.vue";
  import type { StatsFormat } from "./types";
  import { Card } from "@/components/ui/card";

  type Props = {
    title: string;
    value: number;
    subtitle?: string;
    type?: StatsFormat;
    icon?: Component;
  };

  const props = withDefaults(defineProps<Props>(), {
    type: "number",
    subtitle: undefined,
    icon: undefined,
  });

  // Only plain numbers are animated; currency values are formatted by the Currency component
  const numericValue = computed(() => (props.type === "number" ? props.value : 0));
  const displayValue = useCountUp(numericValue);
</script>
