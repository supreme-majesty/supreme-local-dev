import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Container,
  ExternalLink,
  FileText,
  Folder,
  Link2,
  Play,
  Power,
  RefreshCw,
  RotateCw,
  Search,
  Settings2,
  SlidersHorizontal,
  Square,
  Trash2,
  X,
} from "lucide-react";
import { Link, useNavigate } from "react-router-dom";
import {
  ApiError,
  v2,
  type Candidate,
  type JobView,
  type ProjectAction,
  type ProjectUsage,
  type ProjectView,
} from "@/api/v2";
import { UsageText } from "@/components/projects/UsageText";
import { Button } from "@/components/ui/Button";
import { JobConsole } from "@/components/projects/JobConsole";
import { LogViewer } from "@/components/projects/LogViewer";
import { useToast } from "@/hooks/useToast";
import { cn } from "@/lib/utils";

const statusStyle: Record<ProjectView["status"], { label: string; className: string }> = {
  running: { label: "Running", className: "bg-green-500/15 text-green-400 border-green-500/30" },
  partial: { label: "Partly running", className: "bg-amber-500/15 text-amber-400 border-amber-500/30" },
  stopped: { label: "Stopped", className: "bg-slate-500/15 text-slate-300 border-slate-500/30" },
  "not-configured": { label: "Not set up", className: "bg-blue-500/15 text-blue-300 border-blue-500/30" },
  undetected: { label: "Unknown stack", className: "bg-slate-500/10 text-slate-400 border-slate-500/20" },
  invalid: { label: "Config error", className: "bg-red-500/15 text-red-400 border-red-500/30" },
};

const actionLabels: Record<ProjectAction, string> = {
  setup: "Setting up",
  start: "Starting",
  stop: "Stopping",
  restart: "Restarting",
  destroy: "Removing",
};

type Filter = "all" | "running" | "configured" | "not-configured";

export default function ProjectsV2() {
  const queryClient = useQueryClient();
  const { success, error: toastError } = useToast();
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [linkOpen, setLinkOpen] = useState(false);
  const [job, setJob] = useState<{ id: string; title: string } | null>(null);
  const [logsFor, setLogsFor] = useState<ProjectView | null>(null);
  const [confirmDestroy, setConfirmDestroy] = useState<ProjectView | null>(null);
  const [dockerizeOpen, setDockerizeOpen] = useState(false);

  const candidates = useQuery({ queryKey: ["v2", "candidates"], queryFn: () => v2.candidates() });
  const candidateCount = (candidates.data ?? []).filter((c) => c.detected).length;

  const { data: projects, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ["v2", "projects"],
    queryFn: () => v2.projects(),
    refetchInterval: job ? 3000 : 15000,
  });

  // CPU, memory and problems per project (sampled by SLD every few seconds).
  const usage = useQuery({ queryKey: ["v2", "usage"], queryFn: () => v2.usage(), refetchInterval: 10000 });

  // SLD only detects dockerized and parked/linked projects, so everything it returns is listed.
  const base = useMemo(() => projects ?? [], [projects]);

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    return base.filter((p) => {
      if (q && !`${p.name} ${p.folder}`.toLowerCase().includes(q)) return false;
      switch (filter) {
        case "running":
          return p.status === "running" || p.status === "partial";
        case "configured":
          return p.configured;
        case "not-configured":
          return p.status === "not-configured";
        default:
          return true;
      }
    });
  }, [base, filter, search]);

  const counts = useMemo(() => {
    const list = base;
    return {
      all: list.length,
      running: list.filter((p) => p.status === "running" || p.status === "partial").length,
      configured: list.filter((p) => p.configured).length,
      "not-configured": list.filter((p) => p.status === "not-configured").length,
    };
  }, [base]);

  const run = async (project: ProjectView, action: ProjectAction, body?: { remove_volumes?: boolean }) => {
    try {
      const started = await v2.action(project.id, action, body);
      setLogsFor(null);
      setJob({ id: started.id, title: `${actionLabels[action]} ${project.name}` });
      queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && e.jobId) {
        setJob({ id: e.jobId, title: `${project.name} (operation in progress)` });
        return;
      }
      toastError(`Could not ${action} ${project.name}`, (e as Error).message);
    }
  };

  const onJobFinished = (done: JobView) => {
    queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
    queryClient.invalidateQueries({ queryKey: ["v2", "candidates"] });
    if (done.status === "succeeded") success("Done", job?.title);
    else toastError("Operation failed", done.error);
  };

  return (
    <div className="space-y-6 max-w-6xl">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-[var(--foreground)]">Projects</h1>
          <p className="text-sm text-[var(--muted-foreground)]">
            Dockerized projects (their own compose file or Dockerfile, or already running in SLD containers) and
            the folders you parked or linked. Start and stop them from here.
          </p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => setDockerizeOpen(true)}>
            <Container size={16} /> Dockerize projects
            {candidateCount > 0 && (
              <span className="ml-1 rounded-full bg-white/20 px-1.5 text-[11px] leading-5">{candidateCount}</span>
            )}
          </Button>
          <Button variant="secondary" onClick={() => setLinkOpen(true)}>
            <Link2 size={16} /> Park / link folder
          </Button>
          <Button variant="secondary" onClick={() => v2.projects(true).then(() => refetch())} loading={isFetching}>
            <RefreshCw size={16} /> Rescan
          </Button>
        </div>
      </div>

      {dockerizeOpen && (
        <DockerizeDialog
          onClose={() => setDockerizeOpen(false)}
          onStarted={(started, count) => {
            setDockerizeOpen(false);
            setLogsFor(null);
            setJob({ id: started.id, title: `Dockerizing ${count} project${count === 1 ? "" : "s"}` });
          }}
        />
      )}

      {linkOpen && (
        <LinkFolderDialog
          onClose={() => setLinkOpen(false)}
          onLinked={(message) => {
            setLinkOpen(false);
            success("Folder added", message);
            v2.projects(true).then(() => refetch());
          }}
        />
      )}

      <div className="flex flex-wrap items-center gap-2">
        {(["all", "running", "configured", "not-configured"] as Filter[]).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={cn(
              "h-8 px-3 rounded-full text-xs font-medium border transition-colors",
              filter === f
                ? "bg-[var(--primary)] text-[var(--primary-foreground)] border-transparent"
                : "border-[var(--border)] text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
            )}
          >
            {{ all: "All", running: "Running", configured: "Set up", "not-configured": "Not set up" }[f]} ({counts[f]})
          </button>
        ))}
        <div className="relative ml-auto">
          <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter projects"
            className="h-8 w-56 pl-8 pr-3 rounded-lg border border-[var(--border)] bg-[var(--background)] text-sm text-[var(--foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--ring)]"
          />
        </div>
      </div>

      {job && (
        <JobConsole
          key={job.id}
          jobId={job.id}
          title={job.title}
          onClose={() => setJob(null)}
          onFinished={onJobFinished}
        />
      )}
      {logsFor && <LogViewer key={logsFor.id} project={logsFor} onClose={() => setLogsFor(null)} />}

      {isLoading && <p className="text-sm text-[var(--muted-foreground)]">Scanning your workspace…</p>}
      {error && <p className="text-sm text-red-400">Could not load projects: {(error as Error).message}</p>}
      {!isLoading && !error && visible.length === 0 && (
        <div className="rounded-lg border border-dashed border-[var(--border)] p-8 text-center text-sm text-[var(--muted-foreground)]">
          {base.length
            ? "No projects match this filter."
            : "No dockerized projects found. Mount your projects folder into the SLD container (for example -v ~/Projects:/workspace), or park/link a folder of projects without Docker files."}
        </div>
      )}

      <div className="grid gap-3">
        {visible.map((p) => (
          <ProjectRow
            key={p.id || p.folder}
            project={p}
            usage={p.id ? usage.data?.projects[p.id] : undefined}
            busy={Boolean(p.job_id)}
            onAction={(a) => (a === "destroy" ? setConfirmDestroy(p) : run(p, a))}
            onLogs={() => {
              setJob(null);
              setLogsFor(p);
            }}
            onShowJob={() => p.job_id && setJob({ id: p.job_id, title: `${p.name} (in progress)` })}
            onAutostart={async (enabled) => {
              try {
                await v2.setAutostart(p.id, enabled);
                queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
              } catch (e) {
                toastError("Could not change auto start", (e as Error).message);
              }
            }}
            onUnlink={async () => {
              try {
                const res = await v2.setLinked(p.id, false);
                success(
                  res.removed && res.removed !== p.folder ? `Unparked ${res.removed}` : `Unlinked ${p.name}`,
                  p.dockerized ? undefined : "It is no longer detected because it has no Docker files.",
                );
                queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
              } catch (e) {
                toastError("Could not unlink", (e as Error).message);
              }
            }}
          />
        ))}
      </div>

      {confirmDestroy && (
        <DestroyDialog
          project={confirmDestroy}
          onCancel={() => setConfirmDestroy(null)}
          onConfirm={(removeVolumes) => {
            const p = confirmDestroy;
            setConfirmDestroy(null);
            run(p, "destroy", { remove_volumes: removeVolumes });
          }}
        />
      )}
    </div>
  );
}

function ProjectRow({
  project: p,
  usage,
  busy,
  onAction,
  onLogs,
  onShowJob,
  onAutostart,
  onUnlink,
}: {
  project: ProjectView;
  usage?: ProjectUsage;
  busy: boolean;
  onAction: (a: ProjectAction) => void;
  onLogs: () => void;
  onShowJob: () => void;
  onAutostart: (enabled: boolean) => void;
  onUnlink: () => void;
}) {
  const navigate = useNavigate();
  const style = statusStyle[p.status];
  const isUp = p.status === "running" || p.status === "partial";
  const apps = p.apps ?? [];

  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="text-base font-semibold text-[var(--foreground)] truncate">
              {p.id ? (
                <Link to={`/projects/${encodeURIComponent(p.id)}`} className="hover:underline">
                  {p.name}
                </Link>
              ) : (
                p.name
              )}
            </h2>
            <span className={cn("text-[11px] px-2 py-0.5 rounded-full border", style.className)}>{style.label}</span>
            {p.configured && (
              <button
                onClick={() => onAutostart(!p.autostart)}
                title={p.autostart ? "Starts whenever SLD starts — click to turn off" : "Start this project whenever SLD starts"}
                className={cn(
                  "inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full border transition-colors",
                  p.autostart
                    ? "bg-blue-500/15 text-blue-300 border-blue-500/30"
                    : "border-[var(--border)] text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
                )}
              >
                <Power size={11} /> Auto start {p.autostart ? "on" : "off"}
              </button>
            )}
            {p.parked_in && (
              <span
                className="inline-flex items-center gap-1 text-[11px] pl-2 pr-1 py-0.5 rounded-full border border-violet-500/30 bg-violet-500/10 text-violet-300"
                title={p.parked_in === p.folder ? "Linked folder" : `Parked through ${p.parked_in}`}
              >
                <Link2 size={11} /> {p.parked_in === p.folder ? "Linked" : `Parked in ${p.parked_in}`}
                <button
                  onClick={onUnlink}
                  className="rounded-full p-0.5 hover:bg-violet-500/20"
                  title={p.parked_in === p.folder ? "Unlink" : `Unpark ${p.parked_in} (all projects in it)`}
                  aria-label="Unlink"
                >
                  <X size={11} />
                </button>
              </span>
            )}
            {busy && (
              <button onClick={onShowJob} className="text-[11px] text-blue-400 hover:underline">
                operation in progress…
              </button>
            )}
          </div>
          <p className="text-xs text-[var(--muted-foreground)] mt-0.5 flex flex-wrap items-center gap-x-3">
            <span>{p.folder}</span>
            <UsageText usage={usage} />
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          {!p.configured && p.status !== "undetected" && (
            <Button size="sm" onClick={() => onAction("setup")} disabled={busy}>
              <Settings2 size={14} /> Set up
            </Button>
          )}
          {p.configured && !isUp && (
            <Button size="sm" onClick={() => onAction("start")} disabled={busy}>
              <Play size={14} /> Start
            </Button>
          )}
          {p.configured && isUp && (
            <>
              <Button size="sm" variant="secondary" onClick={() => onAction("restart")} disabled={busy}>
                <RotateCw size={14} /> Restart
              </Button>
              <Button size="sm" variant="secondary" onClick={() => onAction("stop")} disabled={busy}>
                <Square size={14} /> Stop
              </Button>
            </>
          )}
          {p.containers > 0 && (
            <Button size="sm" variant="ghost" onClick={onLogs}>
              <FileText size={14} /> Logs
            </Button>
          )}
          {p.id && (
            <Button size="sm" variant="ghost" title="Config, files and environment" onClick={() => navigate(`/projects/${encodeURIComponent(p.id)}`)}>
              <SlidersHorizontal size={14} /> Details
            </Button>
          )}
          {p.configured && (
            <Button size="sm" variant="ghost" onClick={() => onAction("destroy")} disabled={busy} title="Remove containers">
              <Trash2 size={14} />
            </Button>
          )}
        </div>
      </div>

      {apps.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-2">
          {apps.map((a) => (
            <a
              key={a.name}
              href={a.url}
              target="_blank"
              rel="noreferrer"
              className={cn(
                "inline-flex items-center gap-1.5 h-7 px-2.5 rounded-md border text-xs transition-colors",
                isUp
                  ? "border-[var(--border)] text-[var(--foreground)] hover:bg-[var(--card-hover)]"
                  : "border-dashed border-[var(--border)] text-[var(--muted-foreground)] pointer-events-none",
              )}
              aria-disabled={!isUp}
            >
              <span className="font-medium">{a.name}</span>
              <span className="text-[var(--muted-foreground)]">
                {a.runtime}
                {a.version ? ` ${a.version}` : ""}
                {a.service ? ` · ${a.service}` : ""}
              </span>
              <span className="text-blue-400">{a.url.replace(/^https?:\/\//, "")}</span>
              {isUp && <ExternalLink size={12} />}
            </a>
          ))}
        </div>
      )}

      {p.problems && p.problems.length > 0 && (
        <ul className="mt-3 space-y-1">
          {p.problems.map((msg) => (
            <li key={msg} className="flex gap-1.5 text-xs text-amber-400">
              <AlertTriangle size={12} className="mt-0.5 flex-shrink-0" /> {msg}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function DestroyDialog({
  project,
  onCancel,
  onConfirm,
}: {
  project: ProjectView;
  onCancel: () => void;
  onConfirm: (removeVolumes: boolean) => void;
}) {
  const [removeVolumes, setRemoveVolumes] = useState(false);
  const [typed, setTyped] = useState("");
  const confirmed = !removeVolumes || typed === project.name;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true">
      <div className="w-full max-w-md rounded-xl border border-[var(--border)] bg-[var(--card)] p-6 space-y-4">
        <h2 className="text-lg font-semibold text-[var(--foreground)]">Remove {project.name}?</h2>
        <p className="text-sm text-[var(--muted-foreground)]">
          Stops and removes this project&apos;s containers and network. Your code is not touched, and data volumes are
          kept unless you choose otherwise below.
        </p>
        <label className="flex items-start gap-2 text-sm text-[var(--foreground)]">
          <input
            type="checkbox"
            checked={removeVolumes}
            onChange={(e) => setRemoveVolumes(e.target.checked)}
            className="mt-1"
          />
          <span>
            Also delete data volumes (databases, Redis, uploads).{" "}
            <span className="text-red-400">This permanently erases the data.</span>
          </span>
        </label>
        {removeVolumes && (
          <input
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder={`Type ${project.name} to confirm`}
            className="w-full h-9 px-3 rounded-lg border border-red-500/40 bg-[var(--background)] text-sm text-[var(--foreground)] focus:outline-none focus:ring-2 focus:ring-red-500/40"
          />
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="danger" disabled={!confirmed} onClick={() => onConfirm(removeVolumes)}>
            {removeVolumes ? "Remove and delete data" : "Remove containers"}
          </Button>
        </div>
      </div>
    </div>
  );
}

/**
 * Lists workspace projects that have no Docker files, grouped by folder, and dockerizes the
 * selected ones. A project with sub-folder apps (backend/, frontend/...) lets you pick which.
 */
function DockerizeDialog({
  onClose,
  onStarted,
}: {
  onClose: () => void;
  onStarted: (job: JobView, count: number) => void;
}) {
  const { error: toastError } = useToast();
  const list = useQuery({ queryKey: ["v2", "candidates"], queryFn: () => v2.candidates() });
  // Selected projects (by path) and, for each, the chosen app names.
  const [selected, setSelected] = useState<Map<string, Set<string>>>(new Map());
  const [search, setSearch] = useState("");
  const [start, setStart] = useState(false);
  const [saving, setSaving] = useState(false);

  const all = useMemo(() => list.data ?? [], [list.data]);
  const groups = useMemo(() => {
    const q = search.trim().toLowerCase();
    const byGroup = new Map<string, Candidate[]>();
    for (const c of all) {
      const haystack = `${c.name} ${c.folder} ${c.apps.map((a) => `${a.name} ${a.path ?? ""}`).join(" ")}`;
      if (q && !haystack.toLowerCase().includes(q)) continue;
      const slash = c.folder.lastIndexOf("/");
      const group = slash > 0 ? c.folder.slice(0, slash) : "";
      byGroup.set(group, [...(byGroup.get(group) ?? []), c]);
    }
    return [...byGroup.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [all, search]);
  const selectable = groups.flatMap(([, items]) => items.filter((c) => c.detected));

  const allApps = (c: Candidate) => new Set(c.apps.map((a) => a.name));

  const setProjects = (items: Candidate[], on: boolean) =>
    setSelected((prev) => {
      const next = new Map(prev);
      for (const c of items) {
        if (!c.detected) continue;
        if (on) next.set(c.path, next.get(c.path) ?? allApps(c));
        else next.delete(c.path);
      }
      return next;
    });

  const toggleApp = (c: Candidate, app: string) =>
    setSelected((prev) => {
      const next = new Map(prev);
      const apps = new Set(next.get(c.path) ?? []);
      if (apps.has(app)) apps.delete(app);
      else apps.add(app);
      if (apps.size === 0) next.delete(c.path);
      else next.set(c.path, apps);
      return next;
    });

  const submit = async () => {
    setSaving(true);
    try {
      const projects = all
        .filter((c) => selected.has(c.path))
        .map((c) => {
          const apps = selected.get(c.path)!;
          return apps.size === c.apps.length ? { path: c.path } : { path: c.path, apps: [...apps] };
        });
      const job = await v2.dockerize(projects, start);
      onStarted(job, projects.length);
    } catch (e) {
      toastError("Could not dockerize", (e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const stackOf = (a: Candidate["apps"][number]) => `${a.runtime}${a.version ? ` ${a.version}` : ""}`;
  const allChosen = selectable.length > 0 && selectable.every((c) => selected.has(c.path));

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true">
      <div className="w-full max-w-2xl rounded-xl border border-[var(--border)] bg-[var(--card)] p-6 space-y-4">
        <div>
          <h2 className="text-lg font-semibold text-[var(--foreground)]">Dockerize projects</h2>
          <p className="mt-1 text-sm text-[var(--muted-foreground)]">
            Projects in your workspace folders (and their sub-folders) that have no Docker files yet, with or without
            git. For each one you pick, SLD writes a <code className="text-xs">Dockerfile</code> (development and
            production targets), <code className="text-xs">compose.yaml</code>,{" "}
            <code className="text-xs">.dockerignore</code> and a <code className="text-xs">DOCKER.md</code> explaining
            every decision. Everything is based on the stack it detects: pinned versions, non-root users, and a
            database or Redis only when the project uses one. Existing files are never overwritten, and your code and{" "}
            <code className="text-xs">.env</code> are not touched.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-[var(--foreground)]">
            <input
              type="checkbox"
              checked={allChosen}
              onChange={() => setProjects(selectable, !allChosen)}
              disabled={selectable.length === 0}
            />
            Select all
          </label>
          <div className="relative ml-auto">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Filter by name or folder"
              className="h-8 w-56 pl-8 pr-3 rounded-lg border border-[var(--border)] bg-[var(--background)] text-sm text-[var(--foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--ring)]"
            />
          </div>
        </div>

        <div className="max-h-96 overflow-y-auto rounded-md border border-[var(--border)]">
          {list.isLoading && <p className="px-3 py-3 text-xs text-[var(--muted-foreground)]">Scanning your workspace…</p>}
          {list.error && <p className="px-3 py-3 text-xs text-red-400">{(list.error as Error).message}</p>}
          {!list.isLoading && !list.error && groups.length === 0 && (
            <p className="px-3 py-3 text-xs text-[var(--muted-foreground)]">
              {all.length ? "No projects match this filter." : "Every project in your workspace is already dockerized, parked or linked."}
            </p>
          )}
          {groups.map(([group, items]) => {
            const groupSelectable = items.filter((c) => c.detected);
            const groupChosen = groupSelectable.length > 0 && groupSelectable.every((c) => selected.has(c.path));
            return (
              <div key={group || "/"} className="border-b border-[var(--border)] last:border-b-0">
                <label className="flex items-center gap-2 bg-[var(--background)] px-3 py-1.5 text-xs font-medium text-[var(--muted-foreground)]">
                  <input
                    type="checkbox"
                    checked={groupChosen}
                    disabled={groupSelectable.length === 0}
                    onChange={() => setProjects(groupSelectable, !groupChosen)}
                    aria-label={`Select all in ${group || "workspace root"}`}
                  />
                  <Folder size={13} className="text-blue-400" />
                  {group ? `${group}/` : "Workspace root"}
                  <span className="ml-auto">{items.length}</span>
                </label>
                {items.map((c) => {
                  const chosen = selected.get(c.path);
                  const multi = c.apps.length > 1;
                  return (
                    <div
                      key={c.path}
                      className={cn("px-3 py-2 pl-8 text-sm", !c.detected && "opacity-60")}
                      title={c.reason || (c.detected ? undefined : "SLD could not detect this project's stack; add .sld/config.yaml to it by hand")}
                    >
                      <label className={cn("flex items-start gap-3", c.detected && "cursor-pointer")}>
                        <input
                          type="checkbox"
                          className="mt-1"
                          checked={Boolean(chosen)}
                          disabled={!c.detected}
                          onChange={() => setProjects([c], !chosen)}
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-[var(--foreground)] truncate">{c.name}</span>
                            {c.configured && (
                              <span className="text-[10px] px-1.5 py-0.5 rounded-full border border-[var(--border)] text-[var(--muted-foreground)]">
                                has SLD config
                              </span>
                            )}
                          </div>
                          <p className="text-xs text-[var(--muted-foreground)] truncate">{c.folder}</p>
                          {c.reason && <p className="mt-0.5 text-xs text-amber-400">{c.reason}</p>}
                          {!c.reason && !c.detected && <p className="mt-0.5 text-xs text-[var(--muted-foreground)]">Unknown stack</p>}
                        </div>
                        {!multi && c.apps[0] && (
                          <span className="text-xs font-mono whitespace-nowrap text-[var(--foreground)]">{stackOf(c.apps[0])}</span>
                        )}
                      </label>
                      {multi && c.detected && (
                        <div className="mt-1.5 ml-7 flex flex-wrap gap-1.5">
                          {c.apps.map((a) => {
                            const on = chosen?.has(a.name) ?? false;
                            return (
                              <button
                                key={a.name}
                                type="button"
                                onClick={() => toggleApp(c, a.name)}
                                className={cn(
                                  "inline-flex items-center gap-1.5 h-6 px-2 rounded-md border text-xs font-mono transition-colors",
                                  on
                                    ? "border-blue-500/40 bg-blue-500/15 text-blue-200"
                                    : "border-[var(--border)] text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
                                )}
                                title={on ? "Included; click to leave this sub-folder out" : "Click to include this sub-folder"}
                              >
                                {a.path ? `${a.path}/` : a.name} · {stackOf(a)}
                              </button>
                            );
                          })}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>

        <label className="flex items-center gap-2 text-sm text-[var(--foreground)]">
          <input type="checkbox" checked={start} onChange={(e) => setStart(e.target.checked)} />
          Build and start them when done (one after another; this also validates the generated files)
        </label>

        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={selected.size === 0} loading={saving}>
            <Container size={14} /> Dockerize {selected.size || ""} {selected.size === 1 ? "project" : "projects"}
          </Button>
        </div>
      </div>
    </div>
  );
}

/** Picks a folder inside a workspace and parks it (a project, or a folder of projects). */
function LinkFolderDialog({ onClose, onLinked }: { onClose: () => void; onLinked: (message: string) => void }) {
  const queryClient = useQueryClient();
  const { error: toastError } = useToast();
  const system = useQuery({ queryKey: ["v2", "system"], queryFn: () => v2.system() });
  const parked = useQuery({ queryKey: ["v2", "parked"], queryFn: () => v2.parked() });
  const workspaces = system.data?.workspaces ?? [];
  const [root, setRoot] = useState<string | null>(null);
  const [rel, setRel] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const ws = root ?? workspaces[0] ?? "";
  const cwd = [ws, ...rel].join("/");

  const dirs = useQuery({
    queryKey: ["dirs", cwd],
    queryFn: () => v2.directories(cwd),
    enabled: Boolean(ws),
  });

  const submit = async () => {
    setSaving(true);
    try {
      const message = await v2.park(cwd);
      queryClient.invalidateQueries({ queryKey: ["v2", "parked"] });
      onLinked(message);
    } catch (e) {
      toastError("Could not add the folder", (e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const remove = async (path: string) => {
    try {
      await v2.removeParked(path);
      queryClient.invalidateQueries({ queryKey: ["v2", "parked"] });
      queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
    } catch (e) {
      toastError("Could not remove the folder", (e as Error).message);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true">
      <div className="w-full max-w-lg rounded-xl border border-[var(--border)] bg-[var(--card)] p-6 space-y-4">
        <div>
          <h2 className="text-lg font-semibold text-[var(--foreground)]">Park or link a folder</h2>
          <p className="mt-1 text-sm text-[var(--muted-foreground)]">
            SLD only detects dockerized projects on its own. Choose a project folder to link it, or a folder of projects
            to park all of them, so they are detected even without Docker files.
          </p>
        </div>

        {workspaces.length > 1 && (
          <select
            value={ws}
            onChange={(e) => {
              setRoot(e.target.value);
              setRel([]);
            }}
            className="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-sm text-[var(--foreground)]"
            aria-label="Workspace"
          >
            {workspaces.map((w) => (
              <option key={w} value={w}>
                {w}
              </option>
            ))}
          </select>
        )}

        <div className="rounded-md border border-[var(--border)]">
          <div className="flex flex-wrap items-center gap-1 border-b border-[var(--border)] px-3 py-2 text-xs">
            <button className="text-blue-400 hover:underline" onClick={() => setRel([])}>
              {ws || "workspace"}
            </button>
            {rel.map((part, i) => (
              <span key={i} className="flex items-center gap-1">
                <span className="text-[var(--muted-foreground)]">/</span>
                <button className="text-blue-400 hover:underline" onClick={() => setRel(rel.slice(0, i + 1))}>
                  {part}
                </button>
              </span>
            ))}
          </div>
          <div className="max-h-64 overflow-y-auto py-1 text-sm">
            {dirs.isLoading && <p className="px-3 py-2 text-xs text-[var(--muted-foreground)]">Loading…</p>}
            {dirs.error && <p className="px-3 py-2 text-xs text-red-400">{(dirs.error as Error).message}</p>}
            {dirs.data?.length === 0 && <p className="px-3 py-2 text-xs text-[var(--muted-foreground)]">No sub-folders.</p>}
            {dirs.data?.map((d) => (
              <button
                key={d}
                onClick={() => setRel([...rel, d])}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[var(--foreground)] hover:bg-[var(--card-hover)]"
              >
                <Folder size={14} className="text-blue-400" /> {d}
              </button>
            ))}
          </div>
        </div>

        {parked.data && parked.data.length > 0 && (
          <div>
            <p className="mb-1 text-xs font-medium text-[var(--muted-foreground)]">Parked and linked folders</p>
            <ul className="space-y-1">
              {parked.data.map((p) => (
                <li key={p.path} className="flex items-center justify-between gap-2 text-xs">
                  <span className="truncate font-mono text-[var(--foreground)]">{p.folder}</span>
                  <button
                    onClick={() => remove(p.path)}
                    className="rounded p-1 text-[var(--muted-foreground)] hover:bg-[var(--card-hover)] hover:text-red-400"
                    title="Remove"
                  >
                    <X size={12} />
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}

        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={rel.length === 0} loading={saving} title={rel.length === 0 ? "Choose a folder inside the workspace" : undefined}>
            <Link2 size={14} /> Park / link {rel.length ? rel[rel.length - 1] : "folder"}
          </Button>
        </div>
      </div>
    </div>
  );
}
