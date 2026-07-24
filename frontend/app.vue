<template>
  <DialogProvider>
    <ClientOnly>
      <Toaster class="pointer-events-auto" />
    </ClientOnly>

    <NuxtLayout>
      <Html :lang="locale" :data-theme="isDark ? 'dark' : 'light'" :class="{ dark: isDark }" />
      <Link rel="icon" type="image/svg" href="/favicon.svg" />
      <Link rel="apple-touch-icon" href="/apple-touch-icon.png" size="180x180" />
      <Link rel="mask-icon" href="/mask-icon.svg" color="#5048e5" />
      <Meta name="theme-color" :content="isDark ? '#0d121b' : '#5048e5'" />
      <Link rel="manifest" href="/manifest.webmanifest" />
      <NuxtPage />
    </NuxtLayout>
  </DialogProvider>
</template>

<script lang="ts" setup>
  import { useI18n } from "vue-i18n";
  import { DialogProvider } from "@/components/ui/dialog-provider";
  import { Toaster } from "@/components/ui/sonner";

  const { isDark } = useTheme();

  const { locale } = useI18n();

  useViewPreferencesSync();
</script>
