import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import {
  api,
  type SLDState,
  type Project,
  type ServiceStatus,
  type Plugin,
  type Editor,
  type ProjectOptions,
} from "@/api/daemon";
import { useAppStore } from "@/stores/useAppStore";

// Keys
export const queryKeys = {
  state: ["state"],
  sites: ["sites"],
  services: ["services"],
  health: ["health"],
  plugins: ["plugins"],
};

// Queries
export function useSldState() {
  return useQuery<SLDState>({
    queryKey: queryKeys.state,
    queryFn: () => api.getState(),
  });
}

export function useSites() {
  return useQuery<Project[]>({
    queryKey: queryKeys.sites,
    queryFn: () => api.getProjects(),
    refetchInterval: 30000, // Fallback polling (30s)
    refetchIntervalInBackground: true,
  });
}

export function useServices() {
  return useQuery<ServiceStatus[]>({
    queryKey: queryKeys.services,
    queryFn: () => api.getServiceStatus(),
  });
}

export function useHealthChecks() {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: () => api.runDoctor(),
    enabled: false, // Don't run automatically, wait for manual trigger
  });
}

export function usePlugins() {
  return useQuery<Plugin[]>({
    queryKey: queryKeys.plugins,
    queryFn: () => api.getPlugins(),
    refetchInterval: 3000, // frequent polling for status changes
  });
}

// Mutations
export function useParkMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (path: string) => api.park(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.state });
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });
      addToast({ type: "success", title: "Folder Parked" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to park",
        description: err.message,
      });
    },
  });
}

export function useLinkMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: ({ name, path }: { name: string; path: string }) =>
      api.link(name, path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.state });
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });
      addToast({ type: "success", title: "Project Linked" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to link",
        description: err.message,
      });
    },
  });
}

export function useIgnoreMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (path: string) => api.ignore(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });
      addToast({
        type: "success",
        title: "Project Removed",
        description: "Project has been hidden from the list.",
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to remove",
        description: err.message,
      });
    },
  });
}

export function useForgetMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (path: string) => api.forget(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.state });
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });
      addToast({ type: "success", title: "Path Unparked" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to unpark",
        description: err.message,
      });
    },
  });
}

export function useUnlinkMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (name: string) => api.unlink(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.state });
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });
      addToast({ type: "success", title: "Project Unlinked" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to unlink",
        description: err.message,
      });
    },
  });
}

export function useSecureMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: () => api.secure(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.state });
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });
      addToast({
        type: "success",
        title: "HTTPS Enabled",
        description: "SSL certificates installed",
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to enable HTTPS",
        description: err.message,
      });
    },
  });
}

export function useRestartMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: () => api.restart(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.services });
      addToast({ type: "success", title: "Services Restarted" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to restart",
        description: err.message,
      });
    },
  });
}

export function useSwitchPHPMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (version: string) => api.switchPHP(version),
    onSuccess: (_data, version) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.state });
      queryClient.invalidateQueries({ queryKey: queryKeys.services });
      addToast({
        type: "success",
        title: "PHP Switched",
        description: `Now using PHP ${version}`,
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to switch PHP",
        description: err.message,
      });
    },
  });
}

export function useInstallPluginMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (id: string) => api.installPlugin(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.plugins });
      addToast({ type: "success", title: "Plugin installed successfully" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to install plugin",
        description: err.message,
      });
    },
  });
}

export function useTogglePluginMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (args: { id: string; enabled: boolean }) =>
      api.togglePlugin(args.id, args.enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.plugins });
      // Don't toast on every toggle, maybe? Or concise one.
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to toggle plugin",
        description: err.message,
      });
    },
  });
}

export function useEditors() {
  return useQuery<Editor[]>({
    queryKey: ["editors"],
    queryFn: () => api.getEditors(),
  });
}

export function useCreateProjectMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);
  const addPendingProject = useAppStore((s) => s.addPendingProject);

  return useMutation({
    mutationFn: (options: ProjectOptions) => api.createProject(options),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.sites });

      const toastId = addToast({
        type: "info",
        title: "Project Creation Started",
        description: `Creating ${variables.name}... This may take a few minutes.`,
        duration: 0, // Persistent until removed
      });

      addPendingProject(variables.name, variables.type, toastId);
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to create project",
        description: err.message,
      });
    },
  });
}

export function useOpenInEditorMutation() {
  const addToast = useAppStore((s) => s.addToast);
  return useMutation({
    mutationFn: ({ path, editor }: { path: string; editor: string }) =>
      api.openInEditor(path, editor),
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to open editor",
        description: err.message,
      });
    },
  });
}

export function useDirectories(path?: string) {
  return useQuery<string[]>({
    queryKey: ["directories", path],
    queryFn: () => api.getDirectories(path),
    staleTime: 1000 * 60,
  });
}

export function useRealtimeUpdates() {
  const queryClient = useQueryClient();
  const ws = useRef<WebSocket | null>(null);

  useEffect(() => {
    // Determine WS protocol and host
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const url = `${protocol}//${host}/api/ws`;
    let isMounted = true;

    const connect = () => {
      if (!isMounted) return;

      ws.current = new WebSocket(url);

      ws.current.onopen = () => {
        // console.log("SLD: Connected to detailed updates channel");
      };

      ws.current.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "sites:updated") {
            // console.log("SLD: Sites updated event received");
            queryClient.invalidateQueries({ queryKey: queryKeys.sites });
          }
        } catch (e) {
          console.error("SLD: Failed to parse WS message", e);
        }
      };

      ws.current.onclose = () => {
        if (isMounted) {
          setTimeout(connect, 3000);
        }
      };

      ws.current.onerror = (_err) => {
        // console.error("SLD: WS error", err);
        ws.current?.close();
      };
    };

    connect();

    return () => {
      isMounted = false;
      ws.current?.close();
    };
  }, [queryClient]);
}
