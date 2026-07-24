import type { ComputedRef } from "vue";
import type { ThemeMode } from "~/composables/use-preferences";

const VALID_MODES: ThemeMode[] = ["light", "dark", "system"];

export function isValidThemeMode(value: unknown): value is ThemeMode {
  return typeof value === "string" && (VALID_MODES as string[]).includes(value);
}

export interface UseTheme {
  /** The selected mode: light / dark / system (follows the OS). */
  mode: ComputedRef<ThemeMode>;
  /** Resolved dark state after applying the system preference. */
  isDark: ComputedRef<boolean>;
  setTheme: (mode: ThemeMode) => void;
  /** Toggle between explicit light and dark (opts out of "system"). */
  toggleTheme: () => void;
}

export function useTheme(): UseTheme {
  const preferences = useViewPreferences();
  const preferredDark = usePreferredDark();

  // Legacy installs may hold a removed daisyUI theme name — fall back to system.
  const mode = computed<ThemeMode>(() =>
    isValidThemeMode(preferences.value.theme) ? preferences.value.theme : "system"
  );
  const isDark = computed(() => mode.value === "dark" || (mode.value === "system" && preferredDark.value));

  const setTheme = (newMode: ThemeMode) => {
    preferences.value.theme = newMode;
  };

  const toggleTheme = () => {
    setTheme(isDark.value ? "light" : "dark");
  };

  return { mode, isDark, setTheme, toggleTheme };
}
