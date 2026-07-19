import type { ComputedRef, Ref } from "vue";
import { useI18n } from "vue-i18n";
import { scorePassword } from "~~/lib/passwords";

export interface PasswordScore {
  score: ComputedRef<number>;
  message: ComputedRef<string>;
  isValid: ComputedRef<boolean>;
}

export function usePasswordScore(pw: Ref<string>, min = 30): PasswordScore {
  const { t } = useI18n();

  const score = computed(() => {
    return scorePassword(pw.value) || 0;
  });

  const message = computed(() => {
    if (score.value < 20) {
      return t("components.global.password_score.very_weak");
    } else if (score.value < 40) {
      return t("components.global.password_score.weak");
    } else if (score.value < 60) {
      return t("components.global.password_score.good");
    } else if (score.value < 80) {
      return t("components.global.password_score.strong");
    }
    return t("components.global.password_score.very_strong");
  });

  const isValid = computed(() => {
    return score.value >= min;
  });

  return {
    score,
    isValid,
    message,
  };
}
