import { useState, useEffect, useCallback, useRef } from "react";
import { useNavigate } from "react-router-dom";
import {
  Command,
  Search,
  LayoutDashboard,
  FolderOpen,
  Globe,
  Puzzle,
  Zap,
  Share2,
  Settings,
  Stethoscope,
  RefreshCw,
  ExternalLink,
  Terminal,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface CommandItem {
  id: string;
  label: string;
  description?: string;
  icon: React.ElementType;
  category: "navigation" | "php" | "services" | "sites" | "actions";
  action: () => void | Promise<void>;
  keywords?: string[];
}

interface Site {
  name: string;
  domain: string;
}

export function CommandPalette() {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [sites, setSites] = useState<Site[]>([]);
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  // Fetch sites for dynamic commands
  useEffect(() => {
    if (isOpen) {
      fetch("/api/sites")
        .then((res) => res.json())
        .then((data) => setSites(data || []))
        .catch(console.error);
    }
  }, [isOpen]);

  // Define static commands
  const staticCommands: CommandItem[] = [
    // Navigation
    {
      id: "nav-dashboard",
      label: "Go to Dashboard",
      icon: LayoutDashboard,
      category: "navigation",
      action: () => navigate("/"),
      keywords: ["home", "main"],
    },
    {
      id: "nav-projects",
      label: "Go to Projects",
      icon: FolderOpen,
      category: "navigation",
      action: () => navigate("/projects"),
      keywords: ["sites", "folders"],
    },
    {
      id: "nav-domains",
      label: "Go to Domains",
      icon: Globe,
      category: "navigation",
      action: () => navigate("/domains"),
      keywords: ["dns", "hosts"],
    },
    {
      id: "nav-plugins",
      label: "Go to Plugins",
      icon: Puzzle,
      category: "navigation",
      action: () => navigate("/plugins"),
      keywords: ["extensions", "redis", "mailhog"],
    },
    {
      id: "nav-xray",
      label: "Go to X-Ray",
      description: "HTTP Traffic Inspector",
      icon: Zap,
      category: "navigation",
      action: () => navigate("/xray"),
      keywords: ["traffic", "inspector", "logs", "requests"],
    },
    {
      id: "nav-share",
      label: "Go to Share",
      description: "Public tunnels",
      icon: Share2,
      category: "navigation",
      action: () => navigate("/share"),
      keywords: ["tunnel", "cloudflare", "public"],
    },
    {
      id: "nav-settings",
      label: "Go to Settings",
      icon: Settings,
      category: "navigation",
      action: () => navigate("/settings"),
      keywords: ["config", "preferences"],
    },
    {
      id: "nav-doctor",
      label: "Go to Doctor",
      description: "System health check",
      icon: Stethoscope,
      category: "navigation",
      action: () => navigate("/doctor"),
      keywords: ["health", "diagnostics"],
    },

    // PHP Switching
    {
      id: "php-81",
      label: "Switch PHP to 8.1",
      icon: Terminal,
      category: "php",
      action: async () => {
        setLoading(true);
        await fetch("/api/php", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ version: "8.1" }),
        });
        setLoading(false);
      },
      keywords: ["version"],
    },
    {
      id: "php-82",
      label: "Switch PHP to 8.2",
      icon: Terminal,
      category: "php",
      action: async () => {
        setLoading(true);
        await fetch("/api/php", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ version: "8.2" }),
        });
        setLoading(false);
      },
      keywords: ["version"],
    },
    {
      id: "php-83",
      label: "Switch PHP to 8.3",
      icon: Terminal,
      category: "php",
      action: async () => {
        setLoading(true);
        await fetch("/api/php", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ version: "8.3" }),
        });
        setLoading(false);
      },
      keywords: ["version"],
    },

    // Services
    {
      id: "restart-services",
      label: "Restart Services",
      description: "Nginx, PHP, Dnsmasq",
      icon: RefreshCw,
      category: "services",
      action: async () => {
        setLoading(true);
        await fetch("/api/restart", { method: "POST" });
        setLoading(false);
      },
      keywords: ["reload", "nginx", "php"],
    },
  ];

  // Build dynamic site commands
  const siteCommands: CommandItem[] = sites.flatMap((site) => [
    {
      id: `open-${site.name}`,
      label: `Open ${site.name}`,
      description: site.domain,
      icon: ExternalLink,
      category: "sites" as const,
      action: () => {
        window.open(`http://${site.domain}`, "_blank");
      },
      keywords: ["browser", "visit"],
    },
    {
      id: `share-${site.name}`,
      label: `Share ${site.name}`,
      description: "Create public tunnel",
      icon: Share2,
      category: "sites" as const,
      action: async () => {
        setLoading(true);
        await fetch("/api/share/start", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ site: site.name }),
        });
        setLoading(false);
        navigate("/share");
      },
      keywords: ["tunnel", "public"],
    },
    {
      id: `ghost-${site.name}`,
      label: `Fork ${site.name}`,
      description: "Create ghost clone for experimentation",
      icon: FolderOpen,
      category: "actions" as const,
      action: async () => {
        setLoading(true);
        // We need the path, which we don't have in this context
        // Fetch site details to get path
        const res = await fetch("/api/sites");
        const allSites = await res.json();
        const siteData = allSites.find((s: any) => s.name === site.name);
        if (siteData?.path) {
          await fetch("/api/projects/ghost", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              source_path: siteData.path,
              clone_db: true,
            }),
          });
        }
        setLoading(false);
      },
      keywords: ["clone", "ghost", "duplicate", "copy"],
    },
  ]);

  const allCommands = [...staticCommands, ...siteCommands];

  // Filter commands based on search
  const filteredCommands = allCommands.filter((cmd) => {
    if (!search) return true;
    const searchLower = search.toLowerCase();
    return (
      cmd.label.toLowerCase().includes(searchLower) ||
      cmd.description?.toLowerCase().includes(searchLower) ||
      cmd.keywords?.some((k) => k.includes(searchLower))
    );
  });

  // Group by category
  const groupedCommands = filteredCommands.reduce((acc, cmd) => {
    if (!acc[cmd.category]) acc[cmd.category] = [];
    acc[cmd.category].push(cmd);
    return acc;
  }, {} as Record<string, CommandItem[]>);

  const categoryLabels: Record<string, string> = {
    navigation: "Navigation",
    php: "PHP",
    services: "Services",
    sites: "Sites",
    actions: "Actions",
  };

  // Keyboard shortcut to open
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setIsOpen((prev) => !prev);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  // Focus input when opened
  useEffect(() => {
    if (isOpen) {
      setSearch("");
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  // Keyboard navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        setIsOpen(false);
      } else if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((prev) =>
          Math.min(prev + 1, filteredCommands.length - 1)
        );
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((prev) => Math.max(prev - 1, 0));
      } else if (e.key === "Enter" && filteredCommands[selectedIndex]) {
        e.preventDefault();
        executeCommand(filteredCommands[selectedIndex]);
      }
    },
    [filteredCommands, selectedIndex]
  );

  // Execute command
  const executeCommand = async (cmd: CommandItem) => {
    setIsOpen(false);
    await cmd.action();
  };

  // Scroll selected item into view
  useEffect(() => {
    const selected = listRef.current?.querySelector("[data-selected='true']");
    selected?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  // Reset selection when search changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [search]);

  if (!isOpen) return null;

  let flatIndex = -1;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm animate-in fade-in duration-200"
        onClick={() => setIsOpen(false)}
      />

      {/* Palette */}
      <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
        <div
          className="w-full max-w-xl bg-[var(--card)] rounded-2xl border border-[var(--border)] shadow-2xl overflow-hidden animate-in fade-in slide-in-from-top-4 duration-300"
          onKeyDown={handleKeyDown}
        >
          {/* Search Input */}
          <div className="flex items-center gap-3 px-4 py-3 border-b border-[var(--border)]">
            <Search size={20} className="text-[var(--muted-foreground)]" />
            <input
              ref={inputRef}
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Type a command or search..."
              className="flex-1 bg-transparent text-base outline-none placeholder:text-[var(--muted-foreground)]"
            />
            <kbd className="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 text-[10px] font-medium text-[var(--muted-foreground)] bg-[var(--background)] rounded border border-[var(--border)]">
              ESC
            </kbd>
          </div>

          {/* Commands List */}
          <div ref={listRef} className="max-h-[400px] overflow-y-auto p-2">
            {loading && (
              <div className="flex items-center justify-center py-8">
                <div className="w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
              </div>
            )}

            {!loading && filteredCommands.length === 0 && (
              <div className="flex flex-col items-center justify-center py-12 text-[var(--muted-foreground)]">
                <Command
                  size={32}
                  strokeWidth={1}
                  className="mb-2 opacity-50"
                />
                <p className="text-sm">No commands found</p>
              </div>
            )}

            {!loading &&
              Object.entries(groupedCommands).map(([category, commands]) => (
                <div key={category} className="mb-2">
                  <div className="px-2 py-1.5 text-[10px] font-bold uppercase tracking-wider text-[var(--muted-foreground)]">
                    {categoryLabels[category] || category}
                  </div>
                  {commands.map((cmd) => {
                    flatIndex++;
                    const isSelected = flatIndex === selectedIndex;
                    const Icon = cmd.icon;
                    return (
                      <button
                        key={cmd.id}
                        data-selected={isSelected}
                        onClick={() => executeCommand(cmd)}
                        className={cn(
                          "w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-left transition-colors",
                          isSelected
                            ? "bg-[var(--primary)] text-[var(--primary-foreground)]"
                            : "hover:bg-[var(--card-hover)]"
                        )}
                      >
                        <Icon size={18} className="flex-shrink-0 opacity-70" />
                        <div className="flex-1 min-w-0">
                          <div className="text-sm font-medium truncate">
                            {cmd.label}
                          </div>
                          {cmd.description && (
                            <div
                              className={cn(
                                "text-xs truncate",
                                isSelected
                                  ? "text-[var(--primary-foreground)]/70"
                                  : "text-[var(--muted-foreground)]"
                              )}
                            >
                              {cmd.description}
                            </div>
                          )}
                        </div>
                      </button>
                    );
                  })}
                </div>
              ))}
          </div>

          {/* Footer */}
          <div className="flex items-center justify-between px-4 py-2 border-t border-[var(--border)] bg-[var(--background)]">
            <div className="flex items-center gap-4 text-[10px] text-[var(--muted-foreground)]">
              <span className="flex items-center gap-1">
                <kbd className="px-1.5 py-0.5 bg-[var(--card)] rounded border border-[var(--border)]">
                  ↑
                </kbd>
                <kbd className="px-1.5 py-0.5 bg-[var(--card)] rounded border border-[var(--border)]">
                  ↓
                </kbd>
                <span>Navigate</span>
              </span>
              <span className="flex items-center gap-1">
                <kbd className="px-1.5 py-0.5 bg-[var(--card)] rounded border border-[var(--border)]">
                  ↵
                </kbd>
                <span>Select</span>
              </span>
            </div>
            <div className="flex items-center gap-1 text-[10px] text-[var(--muted-foreground)]">
              <Command size={12} />
              <span>Command Palette</span>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
