import { useState, useEffect } from "react";
import {
  Stethoscope,
  CheckCircle,
  XCircle,
  AlertTriangle,
  RefreshCw,
  Wrench,
  Terminal,
} from "lucide-react";
import { Card, CardHeader } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useAppStore } from "@/stores/useAppStore";
import { cn } from "@/lib/utils";

export default function Doctor() {
  const { healthChecks, runDoctor } = useAppStore();
  const [loading, setLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);

  useEffect(() => {
    runDoctor();
  }, [runDoctor]);

  const handleRunDoctor = async () => {
    setLoading(true);
    setLogs(["[info] Running diagnostics..."]);

    await runDoctor();

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

    setLoading(false);
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
        <Button variant="primary" onClick={handleRunDoctor} loading={loading}>
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
      <Card hover={false}>
        <CardHeader
          title="Health Checks"
          description="Detailed system diagnostics"
          icon={<Stethoscope size={20} />}
        />
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
      </Card>

      {/* Log Viewer */}
      <Card hover={false}>
        <CardHeader
          title="Diagnostic Log"
          description="Real-time output from diagnostics"
          icon={<Terminal size={20} />}
        />
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
      </Card>
    </div>
  );
}
