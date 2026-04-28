import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/daemon";
import { useAppStore } from "@/stores/useAppStore";

export const projectFilesKeys = {
  all: ["projectFiles"] as const,
  list: (path: string) => [...projectFilesKeys.all, "list", path] as const,
  content: (path: string) => [...projectFilesKeys.all, "content", path] as const,
};

export function useProjectFiles(path: string | null) {
  return useQuery({
    queryKey: projectFilesKeys.list(path || ""),
    queryFn: () => api.getProjectFiles(path!),
    enabled: !!path,
  });
}

export function useProjectFileContent(path: string | null) {
  return useQuery({
    queryKey: projectFilesKeys.content(path || ""),
    queryFn: () => api.getProjectFileContent(path!),
    enabled: !!path,
  });
}

export function useSaveProjectFileMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { path: string; content: string }) =>
      api.saveProjectFile(vars.path, vars.content),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({
        queryKey: projectFilesKeys.content(vars.path),
      });
      addToast({ type: "success", title: "File saved" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to save file",
        description: err.message,
      });
    },
  });
}
