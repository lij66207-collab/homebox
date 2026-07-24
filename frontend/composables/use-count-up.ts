import type { Ref } from "vue";

/**
 * Animates a numeric display value towards the target with an ease-out cubic
 * curve. Honors prefers-reduced-motion by jumping straight to the target.
 */
export function useCountUp(target: Ref<number>, options?: { duration?: number }): Ref<number> {
  const duration = options?.duration ?? 600;
  const display = ref(0);
  const reducedMotion = usePreferredReducedMotion();

  let raf = 0;

  watch(
    target,
    to => {
      cancelAnimationFrame(raf);

      if (reducedMotion.value === "reduce") {
        display.value = to;
        return;
      }

      const from = display.value;
      const start = performance.now();

      const tick = (now: number) => {
        const progress = Math.min((now - start) / duration, 1);
        const eased = 1 - Math.pow(1 - progress, 3);
        display.value = Math.round(from + (to - from) * eased);
        if (progress < 1) {
          raf = requestAnimationFrame(tick);
        }
      };

      raf = requestAnimationFrame(tick);
    },
    { immediate: true }
  );

  onUnmounted(() => cancelAnimationFrame(raf));

  return display;
}
