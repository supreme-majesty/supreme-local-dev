import { useState } from "react";
import { Puzzle, Settings, Power, Download, ExternalLink } from "lucide-react";
import { Card, CardHeader } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";

// Mock plugin data - will be connected to daemon API later
const mockPlugins = [
  {
    id: "phpmyadmin",
    name: "phpMyAdmin",
    description: "Database management tool for MySQL/MariaDB",
    version: "5.2.1",
    enabled: true,
    icon: "🗄️",
  },
  {
    id: "mailhog",
    name: "MailHog",
    description: "Email testing tool for capturing SMTP emails",
    version: "1.0.1",
    enabled: false,
    icon: "📬",
  },
  {
    id: "redis",
    name: "Redis",
    description: "In-memory data store for caching",
    version: "7.2.3",
    enabled: false,
    icon: "🔴",
  },
  {
    id: "minio",
    name: "MinIO",
    description: "S3-compatible object storage for local development",
    version: "2024.1.1",
    enabled: false,
    icon: "📦",
  },
];

interface Plugin {
  id: string;
  name: string;
  description: string;
  version: string;
  enabled: boolean;
  icon: string;
}

function PluginCard({
  plugin,
  onToggle,
}: {
  plugin: Plugin;
  onToggle: () => void;
}) {
  const [loading, setLoading] = useState(false);

  const handleToggle = async () => {
    setLoading(true);
    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 1000));
    onToggle();
    setLoading(false);
  };

  return (
    <div
      className={cn(
        "p-5 rounded-xl border transition-all duration-200",
        plugin.enabled
          ? "bg-gradient-to-br from-blue-500/5 to-indigo-500/5 border-blue-500/20"
          : "bg-[var(--card)] border-[var(--border)] hover:border-[var(--muted-foreground)]/30"
      )}
    >
      <div className="flex items-start justify-between mb-4">
        <div className="flex items-center gap-3">
          <span className="text-2xl">{plugin.icon}</span>
          <div>
            <h3 className="font-semibold text-[var(--foreground)]">
              {plugin.name}
            </h3>
            <p className="text-xs text-[var(--muted-foreground)] font-mono">
              v{plugin.version}
            </p>
          </div>
        </div>
        <button
          onClick={handleToggle}
          disabled={loading}
          className={cn(
            "relative w-12 h-6 rounded-full transition-colors duration-200",
            plugin.enabled ? "bg-[var(--primary)]" : "bg-[var(--muted)]"
          )}
        >
          <span
            className={cn(
              "absolute top-1 w-4 h-4 rounded-full bg-white shadow transition-transform duration-200",
              plugin.enabled ? "translate-x-7" : "translate-x-1",
              loading && "animate-pulse"
            )}
          />
        </button>
      </div>
      <p className="text-sm text-[var(--muted-foreground)] mb-4">
        {plugin.description}
      </p>
      <div className="flex items-center gap-2">
        <button
          className={cn(
            "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm",
            "bg-[var(--muted)]/50 text-[var(--muted-foreground)]",
            "hover:bg-[var(--muted)] hover:text-[var(--foreground)]",
            "transition-colors duration-200"
          )}
        >
          <Settings size={14} />
          Settings
        </button>
        {plugin.enabled && plugin.id === "phpmyadmin" && (
          <button
            className={cn(
              "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm",
              "bg-[var(--primary)]/10 text-[var(--primary)]",
              "hover:bg-[var(--primary)]/20",
              "transition-colors duration-200"
            )}
            onClick={() => window.open("http://phpmyadmin.test", "_blank")}
          >
            <ExternalLink size={14} />
            Open
          </button>
        )}
      </div>
    </div>
  );
}

export default function Plugins() {
  const [plugins, setPlugins] = useState(mockPlugins);

  const handleToggle = (id: string) => {
    setPlugins((prev) =>
      prev.map((p) => (p.id === id ? { ...p, enabled: !p.enabled } : p))
    );
  };

  const enabledCount = plugins.filter((p) => p.enabled).length;

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Plugins</h1>
          <p className="text-[var(--muted-foreground)]">
            {enabledCount} of {plugins.length} plugins enabled
          </p>
        </div>
        <Button variant="secondary">
          <Download size={16} />
          Browse Plugins
        </Button>
      </div>

      {/* Plugin Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {plugins.map((plugin) => (
          <PluginCard
            key={plugin.id}
            plugin={plugin}
            onToggle={() => handleToggle(plugin.id)}
          />
        ))}
      </div>

      {/* Coming Soon Notice */}
      <Card>
        <div className="text-center py-6">
          <Puzzle
            size={32}
            className="mx-auto mb-3 text-[var(--muted-foreground)] opacity-50"
          />
          <p className="text-[var(--muted-foreground)]">
            More plugins coming soon! Custom plugin development in Phase 2.
          </p>
        </div>
      </Card>
    </div>
  );
}
