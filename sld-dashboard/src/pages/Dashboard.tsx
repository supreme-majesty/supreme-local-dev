import { useState } from "react";
import {
  Server,
  FolderPlus,
  RefreshCw,
  Lock,
  Terminal,
  Cpu,
  HardDrive,
} from "lucide-react";
import {
  useSldState,
  useParkMutation,
  useSecureMutation,
  useRestartMutation,
  useSites,
  usePlugins,
} from "@/hooks/use-daemon";
import { useToast } from "@/hooks/useToast";
import { Card, CardHeader } from "@/components/ui/Card";
import { ServiceCard } from "@/components/dashboard/ServiceCard";
import { QuickActionCard } from "@/components/dashboard/QuickActionCard";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";

import { useServices } from "@/hooks/use-daemon";

export default function Dashboard() {
  const { data: state, isLoading: isStateLoading } = useSldState();
  const { data: services = [], isLoading: isServicesLoading } = useServices();
  const { data: sites = [] } = useSites();
  const { data: plugins = [] } = usePlugins();

  const parkMutation = useParkMutation();
  const secureMutation = useSecureMutation();
  const restartMutation = useRestartMutation();

  const { toast } = useToast();

  const [isParkModalOpen, setIsParkModalOpen] = useState(false);
  const [parkPath, setParkPath] = useState("");

  // Handlers
  const handleParkClick = () => {
    setIsParkModalOpen(true);
  };

  const submitParkFolder = () => {
    if (!parkPath.trim()) return;

    parkMutation.mutate(parkPath, {
      onSuccess: () => {
        setIsParkModalOpen(false);
        setParkPath("");
        toast({ title: "Success", description: "Folder parked successfully" });
      },
    });
  };

  const handleEnableHttps = () => {
    secureMutation.mutate();
  };

  const handleRestartServices = () => {
    restartMutation.mutate();
  };

  // Stats
  const activePlugins = plugins.filter((p) => p.status === "running").length;
  const totalProjects = sites.length;

  const isLoading = isStateLoading || isServicesLoading;

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
      <div>
        <h1 className="text-2xl font-bold gradient-text">Dashboard</h1>
        <p className="text-[var(--muted-foreground)]">
          Manage your local development environment
        </p>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <div className="flex items-center gap-3 p-4 rounded-xl bg-gradient-to-br from-blue-500/10 to-indigo-500/10 border border-blue-500/20">
          <div className="p-2 rounded-lg bg-blue-500/20 text-blue-400">
            <FolderPlus size={20} />
          </div>
          <div>
            <p className="text-2xl font-bold text-[var(--foreground)]">
              {totalProjects}
            </p>
            <p className="text-xs text-[var(--muted-foreground)]">Projects</p>
          </div>
        </div>

        <div className="flex items-center gap-3 p-4 rounded-xl bg-gradient-to-br from-green-500/10 to-emerald-500/10 border border-green-500/20">
          <div className="p-2 rounded-lg bg-green-500/20 text-green-400">
            <Cpu size={20} />
          </div>
          <div>
            <p className="text-2xl font-bold text-[var(--foreground)]">
              {activePlugins}
            </p>
            <p className="text-xs text-[var(--muted-foreground)]">
              Active Plugins
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3 p-4 rounded-xl bg-gradient-to-br from-purple-500/10 to-pink-500/10 border border-purple-500/20">
          <div className="p-2 rounded-lg bg-purple-500/20 text-purple-400">
            <Lock size={20} />
          </div>
          <div>
            <p className="text-2xl font-bold text-[var(--foreground)]">
              {state?.secure ? "On" : "Off"}
            </p>
            <p className="text-xs text-[var(--muted-foreground)]">HTTPS</p>
          </div>
        </div>

        <div className="flex items-center gap-3 p-4 rounded-xl bg-gradient-to-br from-orange-500/10 to-amber-500/10 border border-orange-500/20">
          <div className="p-2 rounded-lg bg-orange-500/20 text-orange-400">
            <Terminal size={20} />
          </div>
          <div>
            <p className="text-2xl font-bold text-[var(--foreground)]">
              PHP {state?.php_version || "?"}
            </p>
            <p className="text-xs text-[var(--muted-foreground)]">Version</p>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Services Section */}
        <Card className="lg:col-span-2">
          <CardHeader
            title="Services"
            description="Core services status"
            icon={<Server size={20} />}
          />
          <div className="space-y-3">
            {services.length > 0 ? (
              services.map((service) => (
                <ServiceCard key={service.name} service={service} />
              ))
            ) : (
              <div className="text-center py-8 text-[var(--muted-foreground)]">
                <Server size={32} className="mx-auto mb-2 opacity-50" />
                <p>Loading services...</p>
              </div>
            )}
          </div>
        </Card>

        {/* Quick Actions */}
        <Card>
          <CardHeader
            title="Quick Actions"
            description="Common tasks"
            icon={<Terminal size={20} />}
          />
          <div className="space-y-3">
            <QuickActionCard
              icon={<FolderPlus size={20} />}
              title="Park Folder"
              description="Register a project directory"
              onClick={handleParkClick}
              variant="primary"
            />
            <QuickActionCard
              icon={<Lock size={20} />}
              title="Enable HTTPS"
              description="Install SSL certificates"
              onClick={handleEnableHttps}
            />
            <QuickActionCard
              icon={<RefreshCw size={20} />}
              title="Restart Services"
              description="Restart Nginx & PHP-FPM"
              onClick={handleRestartServices}
            />
          </div>
        </Card>
      </div>

      {/* Parked Paths Overview */}
      <Card>
        <CardHeader
          title="Parked Paths"
          description={`${state?.paths?.length || 0} directories registered`}
          icon={<HardDrive size={20} />}
        />
        {state?.paths && state.paths.length > 0 ? (
          <div className="space-y-2">
            {state.paths.slice(0, 5).map((path) => (
              <div
                key={path}
                className="flex items-center justify-between p-3 rounded-lg bg-[var(--muted)]/30 border border-[var(--border)]"
              >
                <code className="text-sm font-mono text-[var(--foreground)] truncate">
                  {path}
                </code>
              </div>
            ))}
            {state.paths.length > 5 && (
              <p className="text-sm text-[var(--muted-foreground)] text-center py-2">
                +{state.paths.length - 5} more paths...
              </p>
            )}
          </div>
        ) : (
          <div className="text-center py-8 text-[var(--muted-foreground)]">
            <FolderPlus size={32} className="mx-auto mb-2 opacity-50" />
            <p>No paths parked yet</p>
            <p className="text-sm">Use "Park Folder" to register a directory</p>
          </div>
        )}
      </Card>

      {/* Park Modal */}
      <Modal
        isOpen={isParkModalOpen}
        onClose={() => setIsParkModalOpen(false)}
        title="Park a Directory"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => setIsParkModalOpen(false)}
              disabled={parkMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={submitParkFolder}
              loading={parkMutation.isPending}
              disabled={!parkPath.trim()}
            >
              Park Directory
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <p className="text-sm text-[var(--muted-foreground)]">
            Enter the absolute path to the directory you want to serve projects
            from.
          </p>
          <Input
            placeholder="/home/user/Developments"
            value={parkPath}
            onChange={(e) => setParkPath(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submitParkFolder()}
            autoFocus
          />
        </div>
      </Modal>
    </div>
  );
}
