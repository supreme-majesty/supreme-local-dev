import { Outlet } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { Toast } from "@/components/common/Toast";
import { CommandPalette } from "@/components/CommandPalette";
import { useAppStore } from "@/stores/useAppStore";
import { useRealtimeUpdates } from "@/hooks/use-daemon";
import { cn } from "@/lib/utils";

export function AppShell() {
  const { sidebarCollapsed } = useAppStore();
  useRealtimeUpdates();
  // React Query handles data fetching automatically via hooks in child components
  // We can add global polling here if needed, but per-component useQuery is better

  // Example: Pre-fetch critical data if we want global availability immediately
  // useSldState();
  // useSites();
  // useServices();

  return (
    <div className="min-h-screen bg-[var(--background)]">
      <Sidebar />
      <Topbar />

      {/* Main Content */}
      <main
        className={cn(
          "pt-[var(--topbar-height)] min-h-screen transition-all duration-300 ease-out",
          sidebarCollapsed
            ? "pl-[var(--sidebar-collapsed-width)]"
            : "pl-[var(--sidebar-width)]"
        )}
      >
        <div className="p-6">
          <Outlet />
        </div>
      </main>

      {/* Toast Container */}
      <Toast />

      {/* Command Palette (Cmd+K) */}
      <CommandPalette />
    </div>
  );
}
