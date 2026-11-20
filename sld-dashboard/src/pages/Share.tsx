import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Copy, ExternalLink, Globe, Link2, Loader2, Play, Square, Wifi } from "lucide-react";
import { v2, type ProjectView } from "@/api/v2";
import { useToast } from "@/hooks/useToast";
import { cn } from "@/lib/utils";

// Shape returned by GET /api/share/status.
interface Tunnel {
  site_name: string;
  public_url: string; // empty while cloudflared is still connecting
  started_at: string;
}

async function shareRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/share/${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body as T;
}

const isUp = (p: ProjectView) => p.status === "running" || p.status === "partial";

export default function Share() {
  const queryClient = useQueryClient();
  const { success, error: toastError } = useToast();
  // Several projects can be starting or stopping at the same time.
  const [starting, setStarting] = useState<Set<string>>(new Set());
  const [stopping, setStopping] = useState<Set<string>>(new Set());
  const [copied, setCopied] = useState<string | null>(null);

  const projectsQuery = useQuery({ queryKey: ["v2", "projects"], queryFn: () => v2.projects(), refetchInterval: 15000 });
  const tunnelsQuery = useQuery({
    queryKey: ["share", "status"],
    queryFn: () => shareRequest<Tunnel[]>("status"),
    refetchInterval: 5000,
  });
  const tunnels = useMemo(() => tunnelsQuery.data ?? [], [tunnelsQuery.data]);

  const projects = useMemo(() => {
    const list = (projectsQuery.data ?? []).filter(
      (p) => p.dockerized || tunnels.some((t) => t.site_name === p.name),
    );
    // Running projects (the ones that can be shared) first.
    return [...list].sort((a, b) => Number(isUp(b)) - Number(isUp(a)) || a.name.localeCompare(b.name));
  }, [projectsQuery.data, tunnels]);

  // Tunnels whose project is no longer found (renamed, removed, ignored).
  const orphans = tunnels.filter((t) => !(projectsQuery.data ?? []).some((p) => p.name === t.site_name));

  const mark = (set: typeof setStarting, name: string, on: boolean) =>
    set((prev) => {
      const next = new Set(prev);
      if (on) next.add(name);
      else next.delete(name);
      return next;
    });

  const start = async (name: string) => {
    mark(setStarting, name, true);
    try {
      const res = await shareRequest<{ message: string }>("start", { method: "POST", body: JSON.stringify({ site: name }) });
      success(`${name} is shared`, res.message);
    } catch (e) {
      toastError(`Could not share ${name}`, (e as Error).message);
    } finally {
      mark(setStarting, name, false);
      queryClient.invalidateQueries({ queryKey: ["share", "status"] });
    }
  };

  const stop = async (name: string) => {
    mark(setStopping, name, true);
    try {
      await shareRequest("stop", { method: "POST", body: JSON.stringify({ site: name }) });
    } catch (e) {
      toastError(`Could not stop sharing ${name}`, (e as Error).message);
    } finally {
      mark(setStopping, name, false);
      queryClient.invalidateQueries({ queryKey: ["share", "status"] });
    }
  };

  const copy = async (text: string, name: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(name);
      setTimeout(() => setCopied(null), 2000);
    } catch {
      toastError("Could not copy the URL");
    }
  };

  if (projectsQuery.isLoading || tunnelsQuery.isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center shadow-lg">
          <Globe size={28} className="text-white" />
        </div>
        <div>
          <h1 className="text-2xl font-bold">Share Projects</h1>
          <p className="text-sm text-[var(--muted-foreground)]">
            Give a running project a temporary public URL through a Cloudflare quick tunnel
          </p>
        </div>
      </div>

      {(projectsQuery.error || tunnelsQuery.error) && (
        <p className="text-sm text-red-400">
          {((projectsQuery.error || tunnelsQuery.error) as Error).message}
        </p>
      )}

      {tunnels.length > 0 && (
        <div className="bg-gradient-to-r from-emerald-500/10 to-teal-500/10 border border-emerald-500/20 rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-emerald-500/20 flex items-center justify-center">
              <Wifi size={20} className="text-emerald-500" />
            </div>
            <div>
              <h3 className="font-semibold text-emerald-500">
                {tunnels.length} active tunnel{tunnels.length > 1 ? "s" : ""}
              </h3>
              <p className="text-sm text-[var(--muted-foreground)]">
                Anyone with these URLs can open the projects until you stop sharing them
              </p>
            </div>
          </div>
        </div>
      )}

      <div className="grid gap-4">
        {projects.length === 0 && orphans.length === 0 && (
          <div className="bg-[var(--card)] rounded-xl border border-[var(--border)] p-12 text-center">
            <Globe size={48} className="mx-auto text-[var(--muted-foreground)] opacity-50 mb-4" />
            <h3 className="text-lg font-semibold mb-2">No projects to share</h3>
            <p className="text-sm text-[var(--muted-foreground)]">
              Start a project on the <Link to="/projects" className="text-blue-400 hover:underline">Projects</Link> page
              first.
            </p>
          </div>
        )}

        {projects.map((p) => (
          <ShareCard
            key={p.id || p.name}
            name={p.name}
            subtitle={p.apps?.[0]?.domain ?? p.folder}
            tunnel={tunnels.find((t) => t.site_name === p.name)}
            running={isUp(p)}
            projectId={p.id}
            starting={starting.has(p.name)}
            stopping={stopping.has(p.name)}
            copied={copied === p.name}
            onStart={() => start(p.name)}
            onStop={() => stop(p.name)}
            onCopy={(url) => copy(url, p.name)}
          />
        ))}
        {orphans.map((t) => (
          <ShareCard
            key={`orphan-${t.site_name}`}
            name={t.site_name}
            subtitle="Project no longer found"
            tunnel={t}
            running={false}
            starting={false}
            stopping={stopping.has(t.site_name)}
            copied={copied === t.site_name}
            onStart={() => undefined}
            onStop={() => stop(t.site_name)}
            onCopy={(url) => copy(url, t.site_name)}
          />
        ))}
      </div>

      <div className="bg-blue-500/5 border border-blue-500/20 rounded-xl p-4">
        <h4 className="font-semibold text-blue-500 mb-2">How it works</h4>
        <ul className="text-sm text-[var(--muted-foreground)] space-y-1">
          <li>• Each shared project gets its own tunnel; sharing one never stops another</li>
          <li>• URLs are temporary and change each time you share a project again</li>
          <li>• Tunnels keep running until you stop them, also when you leave this page</li>
          <li>• Only running projects can be shared</li>
        </ul>
      </div>
    </div>
  );
}

function ShareCard({
  name,
  subtitle,
  tunnel,
  running,
  projectId,
  starting,
  stopping,
  copied,
  onStart,
  onStop,
  onCopy,
}: {
  name: string;
  subtitle: string;
  tunnel?: Tunnel;
  running: boolean;
  projectId?: string;
  starting: boolean;
  stopping: boolean;
  copied: boolean;
  onStart: () => void;
  onStop: () => void;
  onCopy: (url: string) => void;
}) {
  const active = Boolean(tunnel);
  return (
    <div
      className={cn(
        "bg-[var(--card)] rounded-xl border p-5 transition-all duration-300",
        active ? "border-emerald-500/50 shadow-lg shadow-emerald-500/5" : "border-[var(--border)]",
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-4 min-w-0">
          <div
            className={cn(
              "w-12 h-12 flex-shrink-0 rounded-xl flex items-center justify-center",
              active ? "bg-emerald-500/10 text-emerald-500" : "bg-[var(--card-hover)] text-[var(--muted-foreground)]",
            )}
          >
            <Link2 size={24} />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h3 className="font-semibold truncate">{name}</h3>
              {active && (
                <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase bg-emerald-500/10 text-emerald-500">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                  Live
                </span>
              )}
              {!active && !running && projectId && (
                <span className="px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-500/15 text-slate-300">
                  Stopped
                </span>
              )}
            </div>
            <p className="text-sm text-[var(--muted-foreground)] truncate">{subtitle}</p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          {active && !tunnel?.public_url && (
            <span className="flex items-center gap-2 text-xs text-[var(--muted-foreground)]">
              <Loader2 size={14} className="animate-spin" /> Connecting…
            </span>
          )}
          {active && tunnel?.public_url && (
            <div className="flex items-center gap-2 bg-[var(--background)] rounded-lg px-3 py-2 border border-[var(--border)]">
              <span className="text-sm font-mono text-emerald-500 max-w-[260px] truncate">{tunnel.public_url}</span>
              <button
                onClick={() => onCopy(tunnel.public_url)}
                className="p-1.5 rounded-md hover:bg-[var(--card-hover)]"
                title="Copy URL"
              >
                {copied ? <CheckCircle2 size={16} className="text-emerald-500" /> : <Copy size={16} className="text-[var(--muted-foreground)]" />}
              </button>
              <a
                href={tunnel.public_url}
                target="_blank"
                rel="noopener noreferrer"
                className="p-1.5 rounded-md hover:bg-[var(--card-hover)]"
                title="Open in new tab"
              >
                <ExternalLink size={16} className="text-[var(--muted-foreground)]" />
              </a>
            </div>
          )}

          {active ? (
            <button
              onClick={onStop}
              disabled={stopping}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-lg font-medium text-sm bg-red-500/10 text-red-500 hover:bg-red-500/20",
                stopping && "opacity-50 cursor-not-allowed",
              )}
            >
              {stopping ? <Loader2 size={16} className="animate-spin" /> : <Square size={16} />}
              {stopping ? "Stopping..." : "Stop"}
            </button>
          ) : running ? (
            <button
              onClick={onStart}
              disabled={starting}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-lg font-medium text-sm bg-blue-500/10 text-blue-500 hover:bg-blue-500/20",
                starting && "opacity-50 cursor-not-allowed",
              )}
            >
              {starting ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
              {starting ? "Starting..." : "Share"}
            </button>
          ) : projectId ? (
            <Link
              to={`/projects/${encodeURIComponent(projectId)}`}
              className="px-4 py-2 rounded-lg text-sm text-[var(--muted-foreground)] border border-[var(--border)] hover:text-[var(--foreground)]"
              title="Only running projects can be shared"
            >
              Start it to share
            </Link>
          ) : null}
        </div>
      </div>
    </div>
  );
}
