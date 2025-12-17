import { useAppStore } from "@/stores/useAppStore";

export function useTheme() {
  const { theme, toggleTheme } = useAppStore();

  return {
    theme,
    toggleTheme,
    isDark: theme === "dark",
    isLight: theme === "light",
  };
}
