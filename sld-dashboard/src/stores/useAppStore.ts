import { create } from "zustand";
import {
  api,
  type SLDState,
  type ServiceStatus,
  type HealthCheck,
  type Project,
} from "@/api/daemon";

interface Toast {
  id: string;
  type: "success" | "error" | "info" | "warning";
  title: string;
  description?: string;
}

interface AppState {
  // SLD State from daemon
  state: SLDState | null;
  isLoading: boolean;
  error: string | null;

  // Services
  services: ServiceStatus[];

  // Projects (derived from state)
  projects: Project[];

  // Health checks
  healthChecks: HealthCheck[];

  // UI State
  theme: "dark" | "light";
  sidebarCollapsed: boolean;

  // Toasts
  toasts: Toast[];

  // Actions
  fetchState: () => Promise<void>;
  fetchServices: () => Promise<void>;
  fetchProjects: () => Promise<void>;
  runDoctor: () => Promise<void>;

  // Mutations
  parkPath: (path: string) => Promise<void>;
  forgetPath: (path: string) => Promise<void>;
  linkProject: (name: string, path: string) => Promise<void>;
  unlinkProject: (name: string) => Promise<void>;
  switchPHP: (version: string) => Promise<void>;
  enableSecure: () => Promise<void>;
  restartServices: () => Promise<void>;

  // UI Actions
  toggleTheme: () => void;
  toggleSidebar: () => void;
  addToast: (toast: Omit<Toast, "id">) => void;
  removeToast: (id: string) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  // Initial state
  state: null,
  isLoading: false,
  error: null,
  services: [],
  projects: [],
  healthChecks: [],
  theme: (localStorage.getItem("theme") as "dark" | "light") || "dark",
  sidebarCollapsed: false,
  toasts: [],

  // Fetch actions
  fetchState: async () => {
    set({ isLoading: true, error: null });
    try {
      const state = await api.getState();
      set({ state, isLoading: false });
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to fetch state";
      set({ error: message, isLoading: false });
      get().addToast({
        type: "error",
        title: "Connection Error",
        description: message,
      });
    }
  },

  fetchServices: async () => {
    try {
      const services = await api.getServiceStatus();
      set({ services });
    } catch (error) {
      console.error("Failed to fetch services:", error);
    }
  },

  fetchProjects: async () => {
    try {
      const projects = await api.getProjects();
      set({ projects });
    } catch (error) {
      console.error("Failed to fetch projects:", error);
    }
  },

  runDoctor: async () => {
    try {
      const healthChecks = await api.runDoctor();
      set({ healthChecks });
    } catch (error) {
      console.error("Failed to run doctor:", error);
    }
  },

  // Mutation actions
  parkPath: async (path: string) => {
    try {
      await api.park(path);
      get().addToast({
        type: "success",
        title: "Path Parked",
        description: path,
      });
      await get().fetchState();
      await get().fetchProjects();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to park path";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

  forgetPath: async (path: string) => {
    try {
      await api.forget(path);
      get().addToast({
        type: "success",
        title: "Path Removed",
        description: path,
      });
      await get().fetchState();
      await get().fetchProjects();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to forget path";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

  linkProject: async (name: string, path: string) => {
    try {
      await api.link(name, path);
      get().addToast({
        type: "success",
        title: "Project Linked",
        description: `${name} → ${path}`,
      });
      await get().fetchState();
      await get().fetchProjects();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to link project";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

  unlinkProject: async (name: string) => {
    try {
      await api.unlink(name);
      get().addToast({
        type: "success",
        title: "Project Unlinked",
        description: name,
      });
      await get().fetchState();
      await get().fetchProjects();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to unlink project";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

  switchPHP: async (version: string) => {
    try {
      await api.switchPHP(version);
      get().addToast({
        type: "success",
        title: "PHP Switched",
        description: `Now using PHP ${version}`,
      });
      await get().fetchState();
      await get().fetchServices();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to switch PHP";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

  enableSecure: async () => {
    try {
      await api.secure();
      get().addToast({
        type: "success",
        title: "HTTPS Enabled",
        description: "SSL certificates installed",
      });
      await get().fetchState();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to enable HTTPS";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

  restartServices: async () => {
    try {
      await api.restart();
      get().addToast({ type: "success", title: "Services Restarted" });
      await get().fetchServices();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to restart services";
      get().addToast({ type: "error", title: "Error", description: message });
    }
  },

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
