import { useState } from "react";
import {
  Stethoscope,
  CheckCircle,
  XCircle,
  AlertTriangle,
  RefreshCw,
  Wrench,
  Terminal,
} from "lucide-react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useHealthChecks } from "@/hooks/use-daemon";
import { cn } from "@/lib/utils";

export default function Doctor() {
  const { data: healthChecks = [], refetch, isFetching } = useHealthChecks();
  const [logs, setLogs] = useState<string[]>([]);

  const handleRunDoctor = async () => {
    setLogs(["[info] Running diagnostics..."]);

    await refetch();

    // Simulate log output
    setLogs((prev) => [
      ...prev,
      "[ok] Checking Nginx configuration...",
      "[ok] Verifying PHP-FPM socket...",
      "[ok] Testing DNS resolution...",
      "[ok] Checking SSL certificates...",
      "[ok] Scanning for port conflicts...",
      "[info] All checks completed!",
    ]);
  };

  const passCount = healthChecks.filter((c) => c.status === "pass").length;
  const failCount = healthChecks.filter((c) => c.status === "fail").length;
  const warnCount = healthChecks.filter((c) => c.status === "warn").length;

  const statusIcon = {
    pass: <CheckCircle size={18} className="text-green-400" />,
    fail: <XCircle size={18} className="text-red-400" />,
    warn: <AlertTriangle size={18} className="text-amber-400" />,
  };

  const statusBg = {
    pass: "bg-green-500/10 border-green-500/20",
    fail: "bg-red-500/10 border-red-500/20",
    warn: "bg-amber-500/10 border-amber-500/20",
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Doctor</h1>
          <p className="text-[var(--muted-foreground)]">
            System health checks and diagnostics
          </p>
        </div>
        <Button
          variant="primary"
          onClick={handleRunDoctor}
          loading={isFetching}
        >
          <RefreshCw size={16} />
          Run Diagnostics
        </Button>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Card className="!p-4">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-green-500/10">
              <CheckCircle size={24} className="text-green-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-[var(--foreground)]">
                {passCount}
              </p>
              <p className="text-sm text-[var(--muted-foreground)]">Passed</p>
            </div>
          </div>
        </Card>
        <Card className="!p-4">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-amber-500/10">
              <AlertTriangle size={24} className="text-amber-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-[var(--foreground)]">
                {warnCount}
              </p>
              <p className="text-sm text-[var(--muted-foreground)]">Warnings</p>
            </div>
          </div>
        </Card>
        <Card className="!p-4">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-red-500/10">
              <XCircle size={24} className="text-red-400" />
            </div>
            <div>
              <p className="text-2xl font-bold text-[var(--foreground)]">
                {failCount}
              </p>
              <p className="text-sm text-[var(--muted-foreground)]">Failed</p>
            </div>
          </div>
        </Card>
      </div>

      {/* Health Checks */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Health Checks</CardTitle>
              <CardDescription>Detailed system diagnostics</CardDescription>
            </div>
            <Stethoscope className="h-4 w-4 text-[var(--muted-foreground)]" />
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {healthChecks.map((check, index) => (
              <div
                key={index}
                className={cn(
                  "flex items-center justify-between p-4 rounded-lg border",
                  statusBg[check.status]
                )}
              >
                <div className="flex items-center gap-3">
                  {statusIcon[check.status]}
                  <div>
                    <p className="font-medium text-[var(--foreground)]">
                      {check.name}
                    </p>
                    <p className="text-sm text-[var(--muted-foreground)]">
                      {check.message}
                    </p>
                  </div>
                </div>
                {check.fixable && check.status === "fail" && (
                  <Button variant="secondary" size="sm">
                    <Wrench size={14} />
                    Fix
                  </Button>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Log Viewer */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Diagnostic Log</CardTitle>
              <CardDescription>
                Real-time output from diagnostics
              </CardDescription>
            </div>
            <Terminal className="h-4 w-4 text-[var(--muted-foreground)]" />
          </div>
        </CardHeader>
        <CardContent>
          <div
            className={cn(
              "h-48 overflow-y-auto rounded-lg p-4",
              "bg-[#0d1117] border border-[var(--border)]",
              "font-mono text-sm"
            )}
          >
            {logs.length > 0 ? (
              logs.map((log, index) => (
                <div key={index} className="py-0.5">
                  <span
                    className={cn(
                      log.includes("[ok]") && "text-green-400",
                      log.includes("[error]") && "text-red-400",
                      log.includes("[warn]") && "text-amber-400",
                      log.includes("[info]") && "text-blue-400"
                    )}
                  >
                    {log}
                  </span>
                </div>
              ))
            ) : (
              <p className="text-[var(--muted-foreground)]">
                Click "Run Diagnostics" to see output...
              </p>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
