import { create } from "zustand";
import {
  applyTheme,
  readPreference,
  resolveTheme,
  savePreference,
  watchTheme,
  type ResolvedTheme,
  type ThemePreference,
} from "@/lib/theme";

interface Toast {
  id: string;
  type: "success" | "error" | "info" | "warning";
  title: string;
  description?: string;
  duration?: number;
}

interface PendingProject {
  name: string;
  type: string;
  startedAt: number;
  toastId?: string;
}

interface AppState {
  // UI State
  /** The theme in effect. */
  theme: ResolvedTheme;
  /** What the viewer chose; "system" follows the OS. */
  themePreference: ThemePreference;
  sidebarCollapsed: boolean;

  // Toasts
  toasts: Toast[];

  // Pending Projects
  pendingProjects: PendingProject[];

  // UI Actions
  setTheme: (pref: ThemePreference) => void;
  /** Switches to the opposite of the current theme (an explicit choice). */
  toggleTheme: () => void;
  toggleSidebar: () => void;
  addToast: (toast: Omit<Toast, "id">) => string;
  removeToast: (id: string) => void;
  addPendingProject: (name: string, type: string, toastId?: string) => void;
  removePendingProject: (name: string) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  // Initial state
  theme: resolveTheme(readPreference()),
  themePreference: readPreference(),
  sidebarCollapsed: false,
  toasts: [],
  pendingProjects: JSON.parse(localStorage.getItem("pendingProjects") || "[]"),

  // UI actions
  setTheme: (pref) => {
    savePreference(pref);
    const theme = resolveTheme(pref);
    applyTheme(theme);
    set({ theme, themePreference: pref });
  },

  toggleTheme: () => {
    get().setTheme(get().theme === "dark" ? "light" : "dark");
  },

  toggleSidebar: () => {
    set({ sidebarCollapsed: !get().sidebarCollapsed });
  },

  addToast: (toast) => {
    const id = Math.random().toString(36).substring(2, 9);
    set({ toasts: [...get().toasts, { ...toast, id }] });

    // Auto-remove after custom duration or default 5 seconds
    // If duration is 0, do not auto-remove (persistent)
    if (toast.duration !== 0) {
      setTimeout(() => {
        get().removeToast(id);
      }, toast.duration || 5000);
    }

    return id;
  },

  removeToast: (id) => {
    set({ toasts: get().toasts.filter((t) => t.id !== id) });
  },

  addPendingProject: (name, type, toastId) => {
    const newPending = [
      ...get().pendingProjects,
      { name, type, startedAt: Date.now(), toastId },
    ];
    set({ pendingProjects: newPending });
    localStorage.setItem("pendingProjects", JSON.stringify(newPending));
  },

  removePendingProject: (name) => {
    const projects = get().pendingProjects;
    const project = projects.find((p) => p.name === name);

    // If we have a linked toast, remove it too
    if (project?.toastId) {
      get().removeToast(project.toastId);
    }

    const newPending = projects.filter((p) => p.name !== name);
    set({ pendingProjects: newPending });
    localStorage.setItem("pendingProjects", JSON.stringify(newPending));
  },
}));

// Apply the theme now, and follow OS changes ("system") and other tabs.
if (typeof window !== "undefined") {
  applyTheme(useAppStore.getState().theme);
  watchTheme((pref) => {
    const theme = resolveTheme(pref);
    applyTheme(theme);
    useAppStore.setState({ theme, themePreference: pref });
  });
}
