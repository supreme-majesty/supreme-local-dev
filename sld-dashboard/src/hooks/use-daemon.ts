import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  api,
  type Project,
  type SLDState,
  type ServiceStatus,
} from "@/api/daemon";
import { useAppStore } from "@/stores/useAppStore";

// Keys
export const queryKeys = {
  state: ["state"],
  sites: ["sites"],
  services: ["services"],
  health: ["health"],
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
