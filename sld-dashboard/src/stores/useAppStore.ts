import { create } from "zustand";

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
  theme: "dark" | "light";
  sidebarCollapsed: boolean;

  // Toasts
  toasts: Toast[];

  // Pending Projects
  pendingProjects: PendingProject[];

  // UI Actions
  toggleTheme: () => void;
  toggleSidebar: () => void;
  addToast: (toast: Omit<Toast, "id">) => string;
  removeToast: (id: string) => void;
  addPendingProject: (name: string, type: string, toastId?: string) => void;
  removePendingProject: (name: string) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  // Initial state
  theme: (localStorage.getItem("theme") as "dark" | "light") || "dark",
  sidebarCollapsed: false,
  toasts: [],
  pendingProjects: JSON.parse(localStorage.getItem("pendingProjects") || "[]"),

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

// Initialize theme on load
if (typeof window !== "undefined") {
  const theme = localStorage.getItem("theme") || "dark";
  document.documentElement.setAttribute("data-theme", theme);
}
