<template>
  <ScannerARView />
</template>

<script setup lang="ts">
  import { useI18n } from "vue-i18n";
  import ScannerARView from "@/components/Scanner/ARView.vue";

  const { t } = useI18n();

  definePageMeta({
    middleware: ["auth"],
    layout: "empty",
  });

  useHead({
    title: "HomeBox | " + t("scanner_ar.title"),
  });
</script>
