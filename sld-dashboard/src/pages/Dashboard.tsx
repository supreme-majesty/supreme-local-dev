import { useNavigate } from "react-router-dom";
import {
  FolderPlus,
  RefreshCw,
  Lock,
  HardDrive,
  Terminal,
  Layers,
  Cpu,
} from "lucide-react";
import {
  useSldState,
  useSites,
  usePlugins,
  useRestartMutation,
} from "@/hooks/use-daemon";
import { useToast } from "@/hooks/useToast";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/Card";
import { QuickActionCard } from "@/components/dashboard/QuickActionCard";
import Metrics from "@/components/Metrics";
import { PendingProjects } from "@/components/common/PendingProjects";

const Dashboard = () => {
  const { data: state, isLoading: isStateLoading } = useSldState();
  const { data: sites } = useSites();
  const { data: plugins } = usePlugins();

  // Calculate stats
  const totalProjects = sites?.length || 0;
  const activePlugins = plugins?.filter((p) => p.status === "running").length || 0;

  const restartMutation = useRestartMutation();
  const navigate = useNavigate();
  const { toast } = useToast();

  const handleRestartServices = () => {
    restartMutation.mutate(undefined, {
      onSuccess: () => toast({ title: "Router restarted", description: "Every project route was reloaded" }),
    });
  };

  if (isStateLoading) {
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

      {/* Overview Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Projects</CardTitle>
            <Layers className="h-4 w-4 text-[var(--muted-foreground)]" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalProjects}</div>
            <p className="text-xs text-[var(--muted-foreground)]">
              active projects
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Active Plugins
            </CardTitle>
            <Cpu className="h-4 w-4 text-[var(--muted-foreground)]" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{activePlugins}</div>
            <p className="text-xs text-[var(--muted-foreground)]">running</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">HTTPS</CardTitle>
            <Lock className="h-4 w-4 text-[var(--muted-foreground)]" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {state?.secure ? "Enabled" : "Disabled"}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">PHP Version</CardTitle>
            <Terminal className="h-4 w-4 text-[var(--muted-foreground)]" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">Per project</div>
            <p className="text-xs text-[var(--muted-foreground)]">
              up to {state?.php_version || "8.4"}, set in each project's config
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Live Metrics */}
      <div className="space-y-2">
        <h2 className="text-lg font-semibold tracking-tight">System Metrics</h2>
        <Metrics />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Workspace folders: every project inside them is found automatically */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>Workspace Folders</CardTitle>
                <CardDescription>
                  Projects in {state?.paths?.length === 1 ? "this folder are" : "these folders are"} found automatically
                </CardDescription>
              </div>
              <HardDrive className="h-4 w-4 text-[var(--muted-foreground)]" />
            </div>
          </CardHeader>
          <CardContent>
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
                <p>No workspace folder</p>
                <p className="text-sm">
                  Mount your projects folder into SLD (-v &lt;projects&gt;:/workspace)
                </p>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Quick Actions */}
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>Quick Actions</CardTitle>
                <CardDescription>Common tasks</CardDescription>
              </div>
              <Terminal className="h-4 w-4 text-[var(--muted-foreground)]" />
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <QuickActionCard
                icon={<FolderPlus size={20} />}
                title="Find Projects"
                description="Set up and start projects in your workspace"
                onClick={() => navigate("/projects")}
                variant="primary"
              />
              <QuickActionCard
                icon={<Lock size={20} />}
                title="Enable HTTPS"
                description="Trust SLD's certificate authority"
                onClick={() => navigate("/setup")}
              />
              <QuickActionCard
                icon={<RefreshCw size={20} />}
                title="Restart Router"
                description="Reload every project route"
                onClick={handleRestartServices}
              />
            </div>
          </CardContent>
        </Card>
      </div>

      <PendingProjects />
    </div>
  );
};

export default Dashboard;
