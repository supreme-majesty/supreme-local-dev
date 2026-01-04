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
  type ProjectTemplate,
  type Tunnel,
} from "@/api/daemon";
import { useAppStore } from "@/stores/useAppStore";
import { useToast } from "@/hooks/useToast";

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

export const useSwitchPHPMutation = () => {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: (version: string) => api.switchPHP(version),
    onSuccess: (_, version) => {
      queryClient.invalidateQueries({ queryKey: ["state"] });
      // Also invalidate services since PHP version changed
      queryClient.invalidateQueries({ queryKey: ["services"] });
      toast({
        title: "PHP Version Switched",
        description: `Successfully switched to PHP ${version}`,
      });
    },
    onError: (error: Error) => {
      toast({
        title: "Switch Failed",
        description: error.message,
        variant: "destructive",
      });
    },
  });
};

export const usePHPVersions = () => {
  return useQuery({
    queryKey: ["php-versions"],
    queryFn: () => api.getPHPVersions(),
    staleTime: 1000 * 60 * 60, // 1 hour
  });
};

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

export function useTemplates() {
  return useQuery<ProjectTemplate[]>({
    queryKey: ["templates"],
    queryFn: () => api.getTemplates(),
    staleTime: 1000 * 60 * 60,
  });
}

export function useDirectories(path?: string) {
  return useQuery<string[]>({
    queryKey: ["directories", path],
    queryFn: () => api.getDirectories(path),
    staleTime: 1000 * 60,
  });
}

// Sharing Hooks
export function useShareStatus() {
  return useQuery<Tunnel[]>({
    queryKey: ["tunnels"],
    queryFn: () => api.getShareStatus(),
    refetchInterval: 5000,
  });
}

export function useShareStartMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (site: string) => api.shareStart(site),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tunnels"] });
      addToast({
        type: "success",
        title: "Site Shared",
        description: "Tunnel started successfully",
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to share site",
        description: err.message,
      });
    },
  });
}

export function useShareStopMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (site: string) => api.shareStop(site),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tunnels"] });
      addToast({ type: "success", title: "Sharing Stopped" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to stop sharing",
        description: err.message,
      });
    },
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
      ws.current?.close();
    };
  }, [queryClient]);
}

export function useArtisanSocket(
  onOutput: (line: string, isError: boolean) => void,
  onDone: (success: boolean) => void,
  projectPath: string | null
) {
  const ws = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!projectPath) return;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const url = `${protocol}//${host}/api/ws`;
    const connect = () => {
      ws.current = new WebSocket(url);

      ws.current.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);

          // Filter by project path if needed, but for now we assume single active console
          if (msg.project_path && msg.project_path !== projectPath) return;

          if (msg.type === "artisan:output") {
            onOutput(msg.line, msg.is_error);
          } else if (msg.type === "artisan:done") {
            onDone(msg.success);
          }
        } catch (e) {
          // ignore
        }
      };

      ws.current.onerror = () => {
        // quiet fail
      };
    };

    connect();

    return () => {
      ws.current?.close();
    };
  }, [projectPath, onOutput, onDone]);
}
// ============ Phase 2 Hooks ============

// Env Manager
export function useEnvFiles(projectPath?: string) {
  return useQuery({
    queryKey: ["env-files", projectPath],
    queryFn: () =>
      projectPath ? api.getEnvFiles(projectPath) : Promise.resolve([]),
    enabled: !!projectPath,
  });
}

export function useEnvFile(path?: string) {
  return useQuery({
    queryKey: ["env-file", path],
    queryFn: () => (path ? api.readEnvFile(path) : Promise.reject("No path")),
    enabled: !!path,
  });
}

export function useSaveEnvMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (args: { path: string; variables: Record<string, string> }) =>
      api.writeEnvFile(args.path, args.variables),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ["env-file", variables.path] });
      queryClient.invalidateQueries({
        queryKey: ["env-backups", variables.path],
      });
      addToast({
        type: "success",
        title: "Env file saved",
        description: "Backup created automatically",
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to save env file",
        description: err.message,
      });
    },
  });
}

export function useEnvBackups(path?: string) {
  return useQuery({
    queryKey: ["env-backups", path],
    queryFn: () => (path ? api.getEnvBackups(path) : Promise.resolve([])),
    enabled: !!path,
  });
}

export function useRestoreEnvBackupMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (args: { backupPath: string; targetPath: string }) =>
      api.restoreEnvBackup(args.backupPath, args.targetPath),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["env-file", variables.targetPath],
      });
      addToast({ type: "success", title: "Backup restored" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to restore backup",
        description: err.message,
      });
    },
  });
}

// Artisan Runner
export function useArtisanRunMutation() {
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (args: { projectPath: string; command: string }) =>
      api.runArtisanCommand(args.projectPath, args.command),
    onSuccess: () => {
      // No toast needed, outcome is streamed
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to start command",
        description: err.message,
      });
    },
  });
}

export function useArtisanCommands() {
  return useQuery({
    queryKey: ["artisan-commands"],
    queryFn: () => api.getArtisanCommands(),
    staleTime: Infinity,
  });
}

// Database Clone
export function useCloneDatabaseMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (args: { source: string; target: string }) =>
      api.cloneDatabase(args.source, args.target),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ["databases"] });
      addToast({
        type: "success",
        title: "Database cloned",
        description: `Cloned ${variables.source} to ${variables.target}`,
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Clone failed",
        description: err.message,
      });
    },
  });
}

// Plugin Logs & Health
export function usePluginLogs(
  id: string,
  lines: number = 100,
  enabled: boolean = false
) {
  return useQuery({
    queryKey: ["plugin-logs", id, lines],
    queryFn: () => api.getPluginLogs(id, lines),
    enabled: enabled && !!id,
    refetchInterval: enabled ? 3000 : false,
    select: (data) => data.logs || [],
  });
}

export function usePluginHealth(id: string) {
  return useQuery({
    queryKey: ["plugin-health", id],
    queryFn: () => api.getPluginHealth(id),
    refetchInterval: 10000,
    enabled: !!id,
  });
}
