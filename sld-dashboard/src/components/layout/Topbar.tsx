import { RefreshCw, Settings, Moon, Sun, Bell, Command } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/stores/useAppStore";
import { useTheme } from "@/hooks/useTheme";
import { StatusBadge } from "@/components/common/StatusBadge";
import { useServices, useRestartMutation } from "@/hooks/use-daemon";
import { Link } from "react-router-dom";
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { v2, type ProjectView } from "@/api/v2";

const MAX_STACKS = 3;

const runtimeNames: Record<string, string> = {
  php: "PHP",
  node: "Node",
  python: "Python",
  ruby: "Ruby",
  go: "Go",
  static: "Static",
  image: "Image",
  compose: "Compose",
};

interface Stack {
  label: string; // e.g. "PHP 8.3"
  projects: string[];
}

// runningStacks lists the runtimes (with versions) of the apps in running
// projects, most used first, e.g. ["PHP 8.3 ×4", "Node 20", "Compose"].
function runningStacks(projects: ProjectView[]): Stack[] {
  const byLabel = new Map<string, Set<string>>();
  for (const p of projects) {
    if (p.status !== "running" && p.status !== "partial") continue;
    for (const a of p.apps ?? []) {
      const name = runtimeNames[a.runtime] ?? a.runtime;
      const label = a.version ? `${name} ${a.version}` : name;
      if (!byLabel.has(label)) byLabel.set(label, new Set());
      byLabel.get(label)!.add(p.name);
    }
  }
  return [...byLabel.entries()]
    .map(([label, names]) => ({ label, projects: [...names].sort() }))
    .sort((a, b) => b.projects.length - a.projects.length || a.label.localeCompare(b.label));
}

export function Topbar() {
  const { sidebarCollapsed } = useAppStore();
  const { data: services = [], isLoading: isServicesLoading } = useServices();
  const { data: projects } = useQuery({
    queryKey: ["v2", "projects"],
    queryFn: () => v2.projects(),
    refetchInterval: 15000,
  });
  const stacks = useMemo(() => runningStacks(projects ?? []), [projects]);
  const restartMutation = useRestartMutation();
  const { toggleTheme, isDark } = useTheme();

  // Check if all services are running
  const allRunning = services.length > 0 && services.every((s) => s.running);
  const isLoading = isServicesLoading;

  const handleRestart = () => {
    restartMutation.mutate();
  };

  const triggerCommandPalette = () => {
    // Dispatch Cmd+K event to trigger the palette
    window.dispatchEvent(
      new KeyboardEvent("keydown", { key: "k", metaKey: true, ctrlKey: true })
    );
  };

  return (
    <header
      className={cn(
        "fixed top-0 right-0 z-30 h-[var(--topbar-height)] bg-[var(--card)] border-b border-[var(--border)]",
        "transition-all duration-300 ease-out flex items-center justify-between px-6",
        sidebarCollapsed
          ? "left-[var(--sidebar-collapsed-width)]"
          : "left-[var(--sidebar-width)]"
      )}
    >
      {/* Left Section - Status */}
      <div className="flex items-center gap-4">
        <StatusBadge
          status={isLoading ? "loading" : allRunning ? "running" : "stopped"}
        />

        {stacks.length > 0 && (
          <div
            className="hidden sm:flex items-center gap-1.5 text-xs text-[var(--muted-foreground)]"
            title={stacks.map((s) => `${s.label}: ${s.projects.join(", ")}`).join("\n")}
          >
            {stacks.slice(0, MAX_STACKS).map((s) => (
              <span
                key={s.label}
                className="font-mono px-2 py-0.5 rounded-md border border-[var(--border)] bg-[var(--background)]"
              >
                {s.label}
                {s.projects.length > 1 && <span className="opacity-60"> ×{s.projects.length}</span>}
              </span>
            ))}
            {stacks.length > MAX_STACKS && <span>+{stacks.length - MAX_STACKS}</span>}
          </div>
        )}

        {/* Command Palette Hint */}
        <button
          onClick={triggerCommandPalette}
          className={cn(
            "hidden md:flex items-center gap-2 px-3 py-1.5 rounded-lg",
            "bg-[var(--background)] border border-[var(--border)]",
            "text-[var(--muted-foreground)] text-sm",
            "hover:border-[var(--border-hover)] hover:text-[var(--foreground)]",
            "transition-all duration-200"
          )}
        >
          <Command size={14} />
          <span>Command</span>
          <kbd className="ml-1 px-1.5 py-0.5 text-[10px] font-medium bg-[var(--card)] rounded border border-[var(--border)]">
            ⌘K
          </kbd>
        </button>
      </div>

      {/* Right Section - Actions */}
      <div className="flex items-center gap-2">
        {/* Restart Button */}
        <button
          onClick={handleRestart}
          disabled={restartMutation.isPending}
          className={cn(
            "flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium",
            "bg-[var(--secondary)] text-[var(--secondary-foreground)]",
            "hover:bg-[var(--card-hover)] transition-colors duration-200",
            "active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed"
          )}
        >
          <RefreshCw
            size={16}
            className={cn(restartMutation.isPending && "animate-spin")}
          />
          <span className="hidden sm:inline">
            {restartMutation.isPending ? "Restarting..." : "Restart"}
          </span>
        </button>

        {/* Theme Toggle */}
        <button
          onClick={toggleTheme}
          className={cn(
            "p-2 rounded-lg",
            "text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
            "hover:bg-[var(--card-hover)] transition-all duration-200",
            "active:scale-95"
          )}
          title={`Switch to ${isDark ? "light" : "dark"} mode`}
          aria-label={`Switch to ${isDark ? "light" : "dark"} mode`}
        >
          {isDark ? <Sun size={20} /> : <Moon size={20} />}
        </button>

        {/* Notifications */}
        <button
          className={cn(
            "p-2 rounded-lg relative",
            "text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
            "hover:bg-[var(--card-hover)] transition-all duration-200"
          )}
          title="Notifications"
        >
          <Bell size={20} />
          {/* Notification dot */}
          {/* <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-[var(--destructive)] rounded-full" /> */}
        </button>

        {/* Settings */}
        <Link
          to="/settings"
          className={cn(
            "p-2 rounded-lg",
            "text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
            "hover:bg-[var(--card-hover)] transition-all duration-200"
          )}
          title="Settings"
        >
          <Settings size={20} />
        </Link>
      </div>
    </header>
  );
}
