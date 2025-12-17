import { create } from "zustand";

interface Toast {
  id: string;
  type: "success" | "error" | "info" | "warning";
  title: string;
  description?: string;
}

interface AppState {
  // UI State
  theme: "dark" | "light";
  sidebarCollapsed: boolean;

  // Toasts
  toasts: Toast[];

  // UI Actions
  toggleTheme: () => void;
  toggleSidebar: () => void;
  addToast: (toast: Omit<Toast, "id">) => void;
  removeToast: (id: string) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  // Initial state
  theme: (localStorage.getItem("theme") as "dark" | "light") || "dark",
  sidebarCollapsed: false,
  toasts: [],

  // UI actions
  toggleTheme: () => {
    const newTheme = get().theme === "dark" ? "light" : "dark";
    localStorage.setItem("theme", newTheme);
    document.documentElement.setAttribute("data-theme", newTheme);
    set({ theme: newTheme });
  },

  toggleSidebar: () => {
    set({ sidebarCollapsed: !get().sidebarCollapsed });
  },

  addToast: (toast) => {
    const id = Math.random().toString(36).substring(2, 9);
    set({ toasts: [...get().toasts, { ...toast, id }] });

    // Auto-remove after 5 seconds
    setTimeout(() => {
      get().removeToast(id);
    }, 5000);
  },

  removeToast: (id) => {
    set({ toasts: get().toasts.filter((t) => t.id !== id) });
  },
}));

// Initialize theme on load
if (typeof window !== "undefined") {
  const theme = localStorage.getItem("theme") || "dark";
  document.documentElement.setAttribute("data-theme", theme);
}
