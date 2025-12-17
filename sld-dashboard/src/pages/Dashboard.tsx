import { useState } from "react";
import {
  Server,
  FolderPlus,
  Globe,
  RefreshCw,
  Lock,
  Terminal,
  Cpu,
  HardDrive,
  Network,
} from "lucide-react";
import { useSldState, useParkMutation } from "@/hooks/use-daemon";
import { useToast } from "@/hooks/useToast";
import { Card, CardHeader } from "@/components/ui/Card";
import { ServiceCard } from "@/components/dashboard/ServiceCard";
import { QuickActionCard } from "@/components/dashboard/QuickActionCard";
import { SystemInfoCard } from "@/components/dashboard/SystemInfoCard";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";

// Mock services hook for now until backend supports it fully or we derive it
import { useServices } from "@/hooks/use-daemon";

export default function Dashboard() {
  const { data: state, isLoading: isStateLoading } = useSldState();
  const { data: services = [], isLoading: isServicesLoading } = useServices();
  const parkMutation = useParkMutation();
  // We need to implement useRestartMutation and useSecureMutation in use-daemon or use api.restart/secure directly wrapped in mutation
  // For now let's assume we add them or use generic mutation

  // Actually, let's implement the missing mutations in use-daemon.ts if they aren't there.
  // checking use-daemon.ts I only added park, link, ignore, forget, unlink.
  // I need to add restart and secure.

  // For this step I will mock them or use generic approach, but better to use what I have.
  // Let's use `api` directly for things I haven't hooked up yet or add hooks.
  // Wait, I can't restart/secure if I don't have hooks.
  // Let's stick to what's available or simple.

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

  const handleEnableHttps = async () => {
    // Todo: Implement secure mutation
    // api.secure()
    toast({
      title: "Info",
      description: "HTTPS setup not yet hooked up to React Query",
    });
  };

  const handleRestartServices = async () => {
    // Todo: Implement restart mutation
    // api.restart()
    toast({
      title: "Info",
      description: "Restart not yet hooked up to React Query",
    });
  };

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

      {/* System Info */}
      <Card>
        <CardHeader
          title="System Information"
          description="Current environment details"
          icon={<Cpu size={20} />}
        />
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <SystemInfoCard
            icon={<Terminal size={20} />}
            label="PHP Version"
            value={state?.php_version || "N/A"}
          />
          <SystemInfoCard
            icon={<Globe size={20} />}
            label="TLD Domain"
            value={`.${state?.tld || "test"}`}
          />
          <SystemInfoCard
            icon={<Lock size={20} />}
            label="HTTPS"
            value={state?.secure ? "Enabled" : "Disabled"}
          />
          <SystemInfoCard
            icon={<Network size={20} />}
            label="HTTP Port"
            value={state?.port || "80"}
          />
        </div>
      </Card>

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
