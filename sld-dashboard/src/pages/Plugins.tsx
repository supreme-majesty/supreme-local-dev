import { useState } from "react";
import { Switch } from "@/components/ui/Switch";
import {
  Puzzle,
  Download,
  ExternalLink,
  RefreshCw,
  ActivitySquare,
} from "lucide-react";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";
import {
  usePlugins,
  useTogglePluginMutation,
  useInstallPluginMutation,
  usePluginHealth,
} from "@/hooks/use-daemon";
import { type Plugin } from "@/api/daemon";
import { PluginLogsModal } from "@/components/dashboard/PluginLogsModal";

function PluginCard({ plugin }: { plugin: Plugin }) {
  const toggleMutation = useTogglePluginMutation();
  const installMutation = useInstallPluginMutation();

  const loading = toggleMutation.isPending || installMutation.isPending;
  const isRunning = plugin.status === "running";

  const handleToggle = () => {
    toggleMutation.mutate({ id: plugin.id, enabled: !isRunning });
  };

  const handleInstall = () => {
    installMutation.mutate(plugin.id);
  };

  // Map ID to icon (since backend doesn't send emojis yet)
  const icons: Record<string, string> = {
    redis: "🔴",
    mailhog: "📬",
    minio: "📦",
    meilisearch: "🔍",
  };

  const { data: health } = usePluginHealth(plugin.id);
  const [showLogs, setShowLogs] = useState(false);

  return (
    <>
      <div
        className={cn(
          "p-5 rounded-xl border transition-all duration-200",
          isRunning
            ? "bg-gradient-to-br from-blue-500/5 to-indigo-500/5 border-blue-500/20"
            : "bg-[var(--card)] border-[var(--border)] hover:border-[var(--muted-foreground)]/30"
        )}
      >
        <div className="flex items-start justify-between mb-4">
          <div className="flex items-center gap-3">
            <span className="text-2xl">{icons[plugin.id] || "🧩"}</span>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="font-semibold text-[var(--foreground)]">
                  {plugin.name}
                </h3>
                {plugin.status === "installing" && (
                  <span className="text-xs text-blue-400 animate-pulse">
                    Installing...
                  </span>
                )}
                {isRunning && health && (
                  <div
                    className={cn(
                      "w-2 h-2 rounded-full",
                      health.healthy ? "bg-green-500" : "bg-red-500"
                    )}
                    title={
                      health.message ||
                      (health.healthy ? "Healthy" : "Unhealthy")
                    }
                  />
                )}
              </div>

              <p className="text-xs text-[var(--muted-foreground)] font-mono">
                v{plugin.version}
              </p>
            </div>
          </div>

          {plugin.installed ? (
            <Switch
              checked={isRunning}
              onCheckedChange={handleToggle}
              loading={loading}
            />
          ) : (
            <Button
              size="sm"
              variant="secondary"
              onClick={handleInstall}
              loading={loading}
            >
              <Download size={14} />
              Install
            </Button>
          )}
        </div>
        <p className="text-sm text-[var(--muted-foreground)] mb-4 h-10 overflow-hidden">
          {plugin.description}
        </p>
        <div className="flex items-center gap-2">
          {plugin.installed && (
            <button
              onClick={() => setShowLogs(true)}
              className={cn(
                "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm",
                "bg-[var(--muted)]/50 text-[var(--muted-foreground)]",
                "hover:bg-[var(--muted)] hover:text-[var(--foreground)]",
                "transition-colors duration-200"
              )}
            >
              <ActivitySquare size={14} />
              Logs
            </button>
          )}

          {plugin.id === "mailhog" && isRunning && (
            <button
              className={cn(
                "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm",
                "bg-[var(--primary)]/10 text-[var(--primary)]",
                "hover:bg-[var(--primary)]/20",
                "transition-colors duration-200"
              )}
              onClick={() => window.open("http://localhost:8025", "_blank")}
            >
              <ExternalLink size={14} />
              Open UI
            </button>
          )}
        </div>
      </div>

      <PluginLogsModal
        isOpen={showLogs}
        onClose={() => setShowLogs(false)}
        pluginId={plugin.id}
        pluginName={plugin.name}
      />
    </>
  );
}

export default function Plugins() {
  const { data: plugins = [], isLoading, refetch } = usePlugins();

  const enabledCount = plugins.filter((p) => p.status === "running").length;

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--primary)] text-[var(--primary)]"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Plugins</h1>
          <p className="text-[var(--muted-foreground)]">
            {enabledCount} active services
          </p>
        </div>
        <Button variant="secondary" onClick={() => refetch()}>
          <RefreshCw size={16} />
          Refresh
        </Button>
      </div>

      {/* Plugin Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {plugins.map((plugin) => (
          <PluginCard key={plugin.id} plugin={plugin} />
        ))}
      </div>

      {plugins.length === 0 && (
        <Card>
          <div className="text-center py-10 text-[var(--muted-foreground)]">
            No plugins found. Is the daemon running?
          </div>
        </Card>
      )}

      {/* Coming Soon Notice */}
      <Card>
        <div className="text-center py-6">
          <Puzzle
            size={32}
            className="mx-auto mb-3 text-[var(--muted-foreground)] opacity-50"
          />
          <p className="text-[var(--muted-foreground)]">
            More plugins coming soon!
          </p>
        </div>
      </Card>
    </div>
  );
}
