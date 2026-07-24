import { useI18n } from "vue-i18n";
import type { Component } from "vue";
import MdiCashMultiple from "~icons/mdi/cash-multiple";
import MdiPackageVariant from "~icons/mdi/package-variant";
import MdiFileTree from "~icons/mdi/file-tree";
import MdiTagMultiple from "~icons/mdi/tag-multiple";
import type { UserClient } from "~~/lib/api/user";

type StatCard = {
  label: string;
  value: number;
  type: "currency" | "number";
  icon: Component;
};

export function statCardData(api: UserClient) {
  const { t } = useI18n();

  const { data: statistics, pending } = useAsyncData(
    "statistics",
    async () => {
      const { data } = await api.stats.group();
      return data;
    },
    {
      deep: true,
    }
  );

  const stats = computed(() => {
    return [
      {
        label: t("home.total_value"),
        value: statistics.value?.totalItemPrice || 0,
        type: "currency",
        icon: MdiCashMultiple,
      },
      {
        label: t("home.total_items"),
        value: statistics.value?.totalItems || 0,
        type: "number",
        icon: MdiPackageVariant,
      },
      {
        label: t("home.total_locations"),
        value: statistics.value?.totalLocations || 0,
        type: "number",
        icon: MdiFileTree,
      },
      {
        label: t("home.total_tags"),
        value: statistics.value?.totalTags || 0,
        type: "number",
        icon: MdiTagMultiple,
      },
    ] as StatCard[];
  });

  return { stats, pending };
}
