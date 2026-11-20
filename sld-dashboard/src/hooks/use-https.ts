import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, v2, type ProjectView } from "@/api/v2";
import { useToast } from "@/hooks/useToast";

/**
 * Turns HTTPS on or off for a project. For a running project the server
 * starts a job that applies it; this follows the job and reports the result.
 */
export function useSetHttps() {
  const queryClient = useQueryClient();
  const { success, info, error: toastError } = useToast();
  const [pending, setPending] = useState<Set<string>>(new Set());

  const mark = (id: string, on: boolean) =>
    setPending((prev) => {
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
    queryClient.invalidateQueries({ queryKey: ["v2", "project"] });
    queryClient.invalidateQueries({ queryKey: ["sites"] });
  };

  const setHttps = async (project: ProjectView, enabled: boolean) => {
    mark(project.id, true);
    const label = enabled ? "on" : "off";
    try {
      const res = await v2.setTLS(project.id, enabled);
      refresh();
      if (!res.job) {
        mark(project.id, false);
        success(`HTTPS ${label} for ${project.name}`, enabled ? "It applies the next time the project starts." : undefined);
        return;
      }
      info(`Applying HTTPS ${label} to ${project.name}…`);
      v2.followJob(
        res.job.id,
        () => undefined,
        (job) => {
          mark(project.id, false);
          refresh();
          if (job.status === "succeeded") success(`HTTPS ${label} for ${project.name}`);
          else toastError(`Could not apply HTTPS to ${project.name}`, job.error);
        },
      );
    } catch (e) {
      mark(project.id, false);
      const msg = (e as Error).message;
      toastError(`Could not turn HTTPS ${label} for ${project.name}`, msg);
      if (e instanceof ApiError && e.status === 409) refresh();
    }
  };

  return { setHttps, pending };
}

export type CaTrust = "trusted" | "untrusted" | "unknown";

/**
 * Checks whether this browser trusts SLD's certificate authority by loading
 * the router's own HTTPS endpoint (its certificate covers "localhost"). An
 * untrusted certificate makes the request fail; any HTTP answer means the
 * certificate was accepted. Rechecked when the window regains focus, so
 * installing the certificate and coming back updates it.
 */
export function useCaTrust(httpsPort?: number, routerRunning?: boolean) {
  return useQuery<CaTrust>({
    queryKey: ["ca-trust", httpsPort],
    enabled: Boolean(httpsPort) && routerRunning === true,
    staleTime: 0,
    refetchOnWindowFocus: true,
    retry: false,
    queryFn: async () => {
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), 4000);
      try {
        await fetch(`https://localhost:${httpsPort}/`, { mode: "no-cors", cache: "no-store", signal: ctrl.signal });
        return "trusted";
      } catch (e) {
        return (e as Error).name === "AbortError" ? "unknown" : "untrusted";
      } finally {
        clearTimeout(timer);
      }
    },
  });
}
