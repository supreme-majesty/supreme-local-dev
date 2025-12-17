import { RefreshCw, Settings, Moon, Sun, Bell } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/stores/useAppStore";
import { useTheme } from "@/hooks/useTheme";
import { StatusBadge } from "@/components/common/StatusBadge";
import { useServices, useRestartMutation } from "@/hooks/use-daemon";
import { Link } from "react-router-dom";

export function Topbar() {
  const { sidebarCollapsed } = useAppStore();
  const { data: services = [], isLoading: isServicesLoading } = useServices();
  const restartMutation = useRestartMutation();
  const { toggleTheme, isDark } = useTheme();

  // Check if all services are running
  const allRunning = services.length > 0 && services.every((s) => s.running);
  const isLoading = isServicesLoading;

  const handleRestart = () => {
    restartMutation.mutate();
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

        {services.length > 0 && (
          <div className="hidden sm:flex items-center gap-2 text-sm text-[var(--muted-foreground)]">
            <span className="font-mono">
              PHP{" "}
              {services.find((s) => s.name.includes("PHP"))?.version || "--"}
            </span>
          </div>
        )}
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
