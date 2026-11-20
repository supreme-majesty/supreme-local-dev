import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Zap } from "lucide-react";
import { v2, type AutomationSettings as Settings } from "@/api/v2";
import { useToast } from "@/hooks/useToast";

const options: { key: keyof Settings; title: string; description: string; warn?: string }[] = [
  {
    key: "auto_setup",
    title: "Set up new projects automatically",
    description: "Projects found in the workspace get their .sld/config.yaml written right away. Nothing is started by this.",
  },
  {
    key: "restore_running",
    title: "Restart what was running",
    description: "After a reboot or when SLD restarts, the projects that were running are started again.",
  },
  {
    key: "autostart_all",
    title: "Start every project with SLD",
    description: "All set-up projects start whenever SLD starts, one after another.",
    warn: "Running many stacks at once needs a lot of memory.",
  },
];

/** Global automation switches; per-project "Auto start" lives on the Projects page. */
export function AutomationSettings() {
  const queryClient = useQueryClient();
  const { error: toastError } = useToast();
  const { data } = useQuery({ queryKey: ["v2", "settings"], queryFn: v2.settings });

  const toggle = async (key: keyof Settings, value: boolean) => {
    try {
      const updated = await v2.updateSettings({ [key]: value });
      queryClient.setQueryData(["v2", "settings"], updated);
      if (key === "auto_setup" && value) queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
    } catch (e) {
      toastError("Could not save setting", (e as Error).message);
    }
  };

  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--card)]">
      <div className="flex items-center gap-2 px-4 py-3 border-b border-[var(--border)]">
        <Zap size={16} className="text-blue-400" />
        <h2 className="text-sm font-semibold text-[var(--foreground)]">Automation</h2>
      </div>
      <div className="divide-y divide-[var(--border)]">
        {options.map((o) => (
          <label key={o.key} className="flex items-start justify-between gap-4 px-4 py-3 cursor-pointer">
            <span>
              <span className="block text-sm font-medium text-[var(--foreground)]">{o.title}</span>
              <span className="block text-xs text-[var(--muted-foreground)]">{o.description}</span>
              {o.warn && data?.[o.key] && <span className="block text-xs text-amber-400 mt-0.5">{o.warn}</span>}
            </span>
            <input
              type="checkbox"
              className="mt-1 h-4 w-4 accent-[var(--primary)]"
              checked={Boolean(data?.[o.key])}
              disabled={!data}
              onChange={(e) => toggle(o.key, e.target.checked)}
            />
          </label>
        ))}
        <p className="px-4 py-3 text-xs text-[var(--muted-foreground)]">
          To start only some projects with SLD, use <span className="text-[var(--foreground)]">Auto start</span> on the
          Projects page. Projects start one at a time; each appears there as a normal start with its output.
        </p>
      </div>
    </div>
  );
}
