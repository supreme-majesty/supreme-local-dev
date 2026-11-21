import { AlertTriangle } from "lucide-react";
import type { ProjectUsage } from "@/api/v2";
import { formatBytes, formatCPU } from "@/lib/format";

/** "CPU 3.2% · 412 MB" plus a problem count, for a project summary. */
export function UsageText({ usage, className }: { usage?: ProjectUsage; className?: string }) {
  if (!usage || (usage.running === 0 && usage.problems === 0)) return null;
  return (
    <span className={className ?? "inline-flex items-center gap-2 text-xs text-[var(--muted-foreground)]"}>
      {usage.running > 0 && (
        <span title="CPU (100% = one core) and memory of the running containers">
          CPU {formatCPU(usage.cpu_percent)} · {formatBytes(usage.mem_bytes)}
        </span>
      )}
      {usage.problems > 0 && (
        <span className="inline-flex items-center gap-1 text-amber-400" title="Containers crash-looping, unhealthy, out of memory or exited with an error">
          <AlertTriangle size={12} /> {usage.problems} {usage.problems === 1 ? "problem" : "problems"}
        </span>
      )}
    </span>
  );
}
