import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Play, RotateCw, Square } from "lucide-react";
import { v2, type ContainerAction, type ContainerHealth, type ProjectView } from "@/api/v2";
import { useToast } from "@/hooks/useToast";
import { cn } from "@/lib/utils";
import { formatBytes, formatCPU } from "@/lib/format";
import { UsageText } from "./UsageText";

interface Props {
  project: ProjectView;
  onChanged: () => void;
}

/**
 * A project's containers with live CPU/memory, restarts, crash loops and the
 * last error line from each log, plus start/stop/restart per container.
 */
export function ContainersHealth({ project, onChanged }: Props) {
  const { error: toastError } = useToast();
  const [pending, setPending] = useState<string | null>(null);
  const hasContainers = (project.services?.length ?? 0) > 0;

  const { data, refetch } = useQuery({
    queryKey: ["v2", "health", project.id],
    queryFn: () => v2.health(project.id),
    enabled: hasContainers,
    refetchInterval: 5000,
  });

  const act = async (name: string, action: ContainerAction) => {
    setPending(name);
    try {
      await v2.containerAction(project.id, name, action);
    } catch (e) {
      toastError(`Could not ${action} ${name}`, (e as Error).message);
    } finally {
      setPending(null);
      onChanged();
      refetch();
    }
  };

  if (!hasContainers) {
    return <p className="text-sm text-[var(--muted-foreground)]">No containers. Start the project to create them.</p>;
  }

  // Until the first health response arrives, show the containers from the project view.
  const containers: ContainerHealth[] =
    data?.containers ??
    (project.services ?? []).map((s) => ({ ...s, restart_count: 0, exit_code: 0 }));

  return (
    <div className="space-y-2">
      {data && <UsageText usage={data.total} />}
      <ul className="divide-y divide-[var(--border)]">
        {containers.map((c) => (
          <ContainerRow key={c.name} c={c} pending={pending} onAction={(a) => act(c.name, a)} />
        ))}
      </ul>
    </div>
  );
}

function ContainerRow({
  c,
  pending,
  onAction,
}: {
  c: ContainerHealth;
  pending: string | null;
  onAction: (a: ContainerAction) => void;
}) {
  const running = c.state === "running";
  const memPct = c.usage && c.usage.mem_limit > 0 ? (c.usage.mem_bytes / c.usage.mem_limit) * 100 : 0;
  return (
    <li className="py-2 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="min-w-0 truncate font-mono text-xs text-[var(--foreground)]">{c.name}</span>
        <span className="flex flex-wrap items-center gap-2 text-xs text-[var(--muted-foreground)]">
          {c.app ? `${c.app} · ` : ""}
          {c.service}
          {c.usage && (
            <span className="tabular-nums" title={`Memory limit ${formatBytes(c.usage.mem_limit)}`}>
              {formatCPU(c.usage.cpu_percent)} CPU · {formatBytes(c.usage.mem_bytes)}
              {memPct >= 80 && <span className="text-amber-400"> ({Math.round(memPct)}% of limit)</span>}
            </span>
          )}
          {c.restart_count > 0 && (
            <span title="Times Docker restarted this container">↻ {c.restart_count}</span>
          )}
          {/* Docker keeps the last healthcheck result after a stop; it only means something while running. */}
          {running && c.health && c.health !== "healthy" && (
            <span className={c.health === "unhealthy" ? "text-red-400" : "text-[var(--muted-foreground)]"}>{c.health}</span>
          )}
          <span
            className={cn(
              "rounded-full border px-2 py-0.5",
              c.problem
                ? "border-red-500/30 bg-red-500/15 text-red-400"
                : running
                  ? "border-green-500/30 bg-green-500/15 text-green-400"
                  : "border-slate-500/30 bg-slate-500/15 text-slate-300",
            )}
          >
            {c.crash_loop ? "crash loop" : c.state}
          </span>
          <span className="flex items-center">
            {running || c.state === "restarting" ? (
              <>
                <IconAction label={`Restart ${c.name}`} disabled={pending !== null} onClick={() => onAction("restart")}>
                  <RotateCw size={13} className={pending === c.name ? "animate-spin" : undefined} />
                </IconAction>
                <IconAction label={`Stop ${c.name}`} disabled={pending !== null} onClick={() => onAction("stop")}>
                  <Square size={13} />
                </IconAction>
              </>
            ) : (
              <IconAction label={`Start ${c.name}`} disabled={pending !== null} onClick={() => onAction("start")}>
                <Play size={13} className={pending === c.name ? "animate-pulse" : undefined} />
              </IconAction>
            )}
          </span>
        </span>
      </div>
      {c.problem && (
        <p className="mt-1 flex items-center gap-1 text-xs text-red-400">
          <AlertTriangle size={12} className="flex-shrink-0" /> {c.problem}
        </p>
      )}
      {c.last_error && (
        <p
          className="mt-1 truncate terminal rounded bg-slate-950/90 px-2 py-1 font-mono text-[11px] text-amber-300"
          title={c.last_error.line}
        >
          {c.last_error.time && (
            <span className="mr-2 text-[var(--muted-foreground)]">{new Date(c.last_error.time).toLocaleTimeString()}</span>
          )}
          {c.last_error.line}
        </p>
      )}
    </li>
  );
}

function IconAction({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
  disabled?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="rounded p-1 text-[var(--muted-foreground)] hover:bg-[var(--card-hover)] hover:text-[var(--foreground)] disabled:opacity-40"
    >
      {children}
    </button>
  );
}
