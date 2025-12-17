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
import { Card, CardHeader } from "@/components/ui/Card";
import { ServiceCard } from "@/components/dashboard/ServiceCard";
import { QuickActionCard } from "@/components/dashboard/QuickActionCard";
import { SystemInfoCard } from "@/components/dashboard/SystemInfoCard";
import { useAppStore } from "@/stores/useAppStore";
import { useToast } from "@/hooks/useToast";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";

export default function Dashboard() {
  const { services, state, parkPath, enableSecure, restartServices } =
    useAppStore();
  const { success } = useToast();
  // State for Modal
  const [isParkModalOpen, setIsParkModalOpen] = useState(false);
  const [parkInput, setParkInput] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Handlers
  const handleParkClick = () => {
    setIsParkModalOpen(true);
  };

  const submitParkFolder = async () => {
    if (!parkInput.trim()) return;

    setIsSubmitting(true);
    try {
      await parkPath(parkInput);
      setParkInput("");
      setIsParkModalOpen(false);
      success("Project parked successfully!");
    } catch (error) {
      console.error(error);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleEnableHttps = async () => {
    await enableSecure();
  };

  const handleRestartServices = async () => {
    await restartServices();
  };

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
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button
              onClick={submitParkFolder}
              loading={isSubmitting}
              disabled={!parkInput.trim()}
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
            value={parkInput}
            onChange={(e) => setParkInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && submitParkFolder()}
            autoFocus
          />
        </div>
      </Modal>
    </div>
  );
}
