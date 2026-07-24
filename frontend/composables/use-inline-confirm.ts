/**
 * Two-step inline confirmation helper for destructive actions.
 *
 * The first `trigger` call arms the confirmation (`confirming` becomes true)
 * and starts a timer that reverts it after `timeout` ms. A second `trigger`
 * call within that window runs `onConfirm`.
 */
export function useInlineConfirm(timeout = 3000) {
  const confirming = ref(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function reset() {
    confirming.value = false;
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
  }

  function trigger(onConfirm: () => void) {
    if (confirming.value) {
      reset();
      onConfirm();
      return;
    }

    confirming.value = true;
    timer = setTimeout(reset, timeout);
  }

  onScopeDispose(reset);

  return { confirming, trigger, reset };
}
