import { useEffect } from "react";
import { Outlet } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { Toast } from "@/components/common/Toast";
import { useAppStore } from "@/stores/useAppStore";
import { cn } from "@/lib/utils";

export function AppShell() {
  const { sidebarCollapsed, fetchState, fetchServices, fetchProjects } =
    useAppStore();

  // Fetch initial data
  useEffect(() => {
    fetchState();
    fetchServices();
    fetchProjects();

    // Poll for updates every 10 seconds
    const interval = setInterval(() => {
      fetchServices();
    }, 10000);

    return () => clearInterval(interval);
  }, [fetchState, fetchServices, fetchProjects]);

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
    </div>
  );
}
