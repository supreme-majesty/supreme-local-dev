import { useAppStore } from "@/stores/useAppStore";
import { Loader2 } from "lucide-react";
import { useSites } from "@/hooks/use-daemon";
import { useEffect } from "react";
import { useToast } from "@/hooks/useToast";

export function PendingProjects() {
  const { pendingProjects, removePendingProject } = useAppStore();
  const { data: sites } = useSites();
  const { success } = useToast();

  useEffect(() => {
    if (!sites || pendingProjects.length === 0) return;

    pendingProjects.forEach((project) => {
      // Check if project name exists in sites list
      const isComplete = sites.some(
        (site: any) =>
          site.name === project.name || site.path?.endsWith("/" + project.name)
      );

      if (isComplete) {
        success(
          "Project Ready",
          `${project.name} has been created successfully.`
        );
        removePendingProject(project.name);
      }
    });
  }, [sites, pendingProjects, removePendingProject, success]);

  if (pendingProjects.length === 0) return null;

  return (
    <div className="fixed bottom-4 left-4 flex flex-col gap-2 z-50">
      {pendingProjects.map((project) => (
        <div
          key={project.name}
          className="bg-[var(--card)] border border-[var(--border)] shadow-lg rounded-lg p-3 flex items-center gap-3 animate-in slide-in-from-bottom-2"
        >
          <div className="relative">
            <div className="absolute inset-0 bg-[var(--primary)]/20 rounded-full animate-ping" />
            <div className="relative bg-[var(--background)] rounded-full p-1">
              <Loader2 className="w-4 h-4 text-[var(--primary)] animate-spin" />
            </div>
          </div>
          <div className="flex flex-col">
            <span className="text-sm font-medium">
              Creating {project.name}...
            </span>
            <span className="text-xs text-[var(--muted-foreground)]">
              This runs in the background
            </span>
          </div>
          <button
            onClick={() => removePendingProject(project.name)}
            className="ml-2 p-1 hover:bg-[var(--muted)] rounded"
          >
            <span className="sr-only">Dismiss</span>×
          </button>
        </div>
      ))}
    </div>
  );
}
