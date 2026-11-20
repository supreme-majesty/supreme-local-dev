import { Settings, Moon, Sun, Command } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/stores/useAppStore";
import { useTheme } from "@/hooks/useTheme";
import { StatusBadge } from "@/components/common/StatusBadge";
import { v2 } from "@/api/v2";

export function Topbar() {
  const { sidebarCollapsed } = useAppStore();
  const { toggleTheme, isDark } = useTheme();
  const { data: sys, isLoading } = useQuery({
    queryKey: ["v2", "system"],
    queryFn: v2.system,
    refetchInterval: 15000,
  });

  // SLD is healthy when it can reach Docker and its router is serving domains.
  const healthy = Boolean(sys?.docker && sys.router === "running");

  const triggerCommandPalette = () => {
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
      <div className="flex items-center gap-4">
        <Link to="/setup" title="SLD status">
          <StatusBadge status={isLoading ? "loading" : healthy ? "running" : "stopped"} />
        </Link>

        {sys && (
          <span className="hidden sm:inline text-sm text-[var(--muted-foreground)]">
            {sys.router === "running"
              ? `Domains on *.${sys.tld}${sys.http_port === 80 ? "" : `:${sys.http_port}`}`
              : "Router not running — open Setup"}
          </span>
        )}

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

      <div className="flex items-center gap-2">
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

        <Link
          to="/setup"
          className={cn(
            "p-2 rounded-lg",
            "text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
            "hover:bg-[var(--card-hover)] transition-all duration-200"
          )}
          title="Setup"
        >
          <Settings size={20} />
        </Link>
      </div>
    </header>
  );
}
