import { NavLink } from "react-router-dom";
import {
  LayoutDashboard,
  FolderOpen,
  Globe,
  Puzzle,
  Settings,
  Stethoscope,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/stores/useAppStore";

const navItems = [
  { to: "/", icon: LayoutDashboard, label: "Dashboard" },
  { to: "/projects", icon: FolderOpen, label: "Projects" },
  { to: "/domains", icon: Globe, label: "Domains" },
  { to: "/plugins", icon: Puzzle, label: "Plugins" },
  { to: "/settings", icon: Settings, label: "Settings" },
  { to: "/doctor", icon: Stethoscope, label: "Doctor" },
];

export function Sidebar() {
  const { sidebarCollapsed, toggleSidebar } = useAppStore();

  return (
    <aside
      className={cn(
        "fixed left-0 top-0 z-40 h-screen flex flex-col bg-[var(--card)] border-r border-[var(--border)]",
        "transition-all duration-300 ease-out",
        sidebarCollapsed
          ? "w-[var(--sidebar-collapsed-width)]"
          : "w-[var(--sidebar-width)]"
      )}
    >
      {/* Logo */}
      <div className="flex items-center gap-3 h-[var(--topbar-height)] px-4 border-b border-[var(--border)]">
        <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center flex-shrink-0">
          <span className="text-white font-bold text-sm">S</span>
        </div>
        {!sidebarCollapsed && (
          <span className="font-semibold text-sm truncate animate-fade-in">
            Supreme Local Dev
          </span>
        )}
      </div>

      {/* Navigation */}
      <nav className="flex-1 py-4 overflow-y-auto">
        <ul className="space-y-1 px-2">
          {navItems.map(({ to, icon: Icon, label }) => (
            <li key={to}>
              <NavLink
                to={to}
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-3 px-3 py-2.5 rounded-lg",
                    "transition-all duration-200 group",
                    "hover:bg-[var(--card-hover)]",
                    isActive
                      ? "bg-[var(--primary)] text-[var(--primary-foreground)] shadow-md"
                      : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                  )
                }
              >
                <Icon
                  size={20}
                  className="flex-shrink-0 transition-transform duration-200 group-hover:scale-110"
                />
                {!sidebarCollapsed && (
                  <span className="text-sm font-medium truncate animate-fade-in">
                    {label}
                  </span>
                )}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>

      {/* Collapse Toggle */}
      <div className="p-2 border-t border-[var(--border)]">
        <button
          onClick={toggleSidebar}
          className={cn(
            "w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg",
            "text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
            "hover:bg-[var(--card-hover)] transition-colors duration-200"
          )}
          title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {sidebarCollapsed ? (
            <ChevronRight size={20} />
          ) : (
            <ChevronLeft size={20} />
          )}
          {!sidebarCollapsed && <span className="text-sm">Collapse</span>}
        </button>
      </div>
    </aside>
  );
}
