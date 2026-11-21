import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Editor from "@monaco-editor/react";
import {
  AlertTriangle,
  ArrowLeft,
  ChevronRight,
  Eye,
  EyeOff,
  ExternalLink,
  File,
  FilePlus,
  Folder,
  History,
  Play,
  Plus,
  RefreshCw,
  RotateCw,
  Save,
  Settings2,
  Square,
  Trash2,
} from "lucide-react";
import {
  ApiError,
  v2,
  type ContainerAction,
  type EnvVar,
  type FileContent,
  type JobView,
  type ProjectAction,
  type ProjectView,
} from "@/api/v2";
import { Button } from "@/components/ui/Button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import { JobConsole } from "@/components/projects/JobConsole";
import { LogViewer } from "@/components/projects/LogViewer";
import { TasksPanel } from "@/components/projects/TasksPanel";
import { CaTrustNotice, HttpsSwitch } from "@/components/projects/HttpsCard";
import { useSetHttps } from "@/hooks/use-https";
import { useToast } from "@/hooks/useToast";
import { useTheme } from "@/hooks/useTheme";
import { cn } from "@/lib/utils";

type Tab = "overview" | "tasks" | "config" | "files" | "env" | "logs";

const actionLabels: Record<ProjectAction, string> = {
  setup: "Setting up",
  start: "Starting",
  stop: "Stopping",
  restart: "Restarting",
  destroy: "Removing",
};

export default function ProjectDetail() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const { success, error: toastError } = useToast();
  const [tab, setTab] = useState<Tab>("overview");
  const [job, setJob] = useState<{ id: string; title: string } | null>(null);

  const { data, isLoading, error } = useQuery({
    queryKey: ["v2", "project", id],
    queryFn: () => v2.project(id),
    refetchInterval: job ? 3000 : 15000,
  });
  const project = data?.project;

  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["v2", "project", id] });
    queryClient.invalidateQueries({ queryKey: ["v2", "projects"] });
  }, [queryClient, id]);

  const run = async (action: ProjectAction) => {
    if (!project) return;
    try {
      const started = await v2.action(project.id, action);
      setJob({ id: started.id, title: `${actionLabels[action]} ${project.name}` });
      refresh();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && e.jobId) {
        setJob({ id: e.jobId, title: `${project.name} (operation in progress)` });
        return;
      }
      toastError(`Could not ${action} ${project.name}`, (e as Error).message);
    }
  };

  const onJobFinished = (done: JobView) => {
    refresh();
    if (done.status === "succeeded") success("Done", job?.title);
    else toastError("Operation failed", done.error);
  };

  if (isLoading) return <p className="text-sm text-[var(--muted-foreground)]">Loading project…</p>;
  if (error || !project) {
    return (
      <div className="space-y-3">
        <BackLink />
        <p className="text-sm text-red-400">Could not load this project: {(error as Error)?.message ?? "not found"}</p>
      </div>
    );
  }

  const isUp = project.status === "running" || project.status === "partial";
  const busy = Boolean(project.job_id);

  return (
    <div className="space-y-5 max-w-6xl">
      <BackLink />
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold text-[var(--foreground)] truncate">{project.name}</h1>
          <p className="text-sm text-[var(--muted-foreground)]">
            {project.folder} · <StatusText project={project} />
          </p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {!project.configured && project.status !== "undetected" && (
            <Button size="sm" onClick={() => run("setup")} disabled={busy}>
              <Settings2 size={14} /> Set up
            </Button>
          )}
          {project.configured && !isUp && (
            <Button size="sm" onClick={() => run("start")} disabled={busy}>
              <Play size={14} /> Start
            </Button>
          )}
          {project.configured && isUp && (
            <>
              <Button size="sm" variant="secondary" onClick={() => run("restart")} disabled={busy}>
                <RotateCw size={14} /> Restart
              </Button>
              <Button size="sm" variant="secondary" onClick={() => run("stop")} disabled={busy}>
                <Square size={14} /> Stop
              </Button>
            </>
          )}
        </div>
      </div>

      {job && (
        <JobConsole key={job.id} jobId={job.id} title={job.title} onClose={() => setJob(null)} onFinished={onJobFinished} />
      )}

      <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="tasks">Tasks</TabsTrigger>
          <TabsTrigger value="config">Config</TabsTrigger>
          <TabsTrigger value="files">Files</TabsTrigger>
          <TabsTrigger value="env">Environment</TabsTrigger>
          <TabsTrigger value="logs" disabled={project.containers === 0}>
            Logs
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <Overview project={project} onChanged={refresh} />
        </TabsContent>
        <TabsContent value="tasks" className="mt-4">
          <TasksPanel project={project} />
        </TabsContent>
        <TabsContent value="config" className="mt-4">
          <ConfigEditor
            project={project}
            config={data?.config}
            onSaved={refresh}
            onRestart={isUp ? () => run("restart") : undefined}
            onSetup={() => run("setup")}
          />
        </TabsContent>
        <TabsContent value="files" className="mt-4">
          <FileBrowser projectId={project.id} onConfigSaved={refresh} />
        </TabsContent>
        <TabsContent value="env" className="mt-4">
          <EnvEditor projectId={project.id} onRestart={isUp ? () => run("restart") : undefined} />
        </TabsContent>
        <TabsContent value="logs" className="mt-4">
          <LogViewer project={project} onClose={() => setTab("overview")} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      to="/projects"
      className="inline-flex items-center gap-1 text-sm text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
    >
      <ArrowLeft size={14} /> Projects
    </Link>
  );
}

function StatusText({ project }: { project: ProjectView }) {
  const labels: Record<ProjectView["status"], string> = {
    running: "running",
    partial: `${project.running} of ${project.containers} containers running`,
    stopped: "stopped",
    "not-configured": "not set up",
    undetected: "stack not recognised",
    invalid: "config error",
  };
  return <span>{labels[project.status]}</span>;
}

// ---------------------------------------------------------------- overview

function Overview({ project, onChanged }: { project: ProjectView; onChanged: () => void }) {
  const isUp = project.status === "running" || project.status === "partial";
  const { error: toastError } = useToast();
  const [pending, setPending] = useState<string | null>(null);
  const containerAction = async (name: string, action: ContainerAction) => {
    setPending(name);
    try {
      await v2.containerAction(project.id, name, action);
    } catch (e) {
      toastError(`Could not ${action} ${name}`, (e as Error).message);
    } finally {
      setPending(null);
      onChanged();
    }
  };
  const apps = project.apps ?? [];
  const services = project.services ?? [];
  return (
    <div className="space-y-4">
      {project.problems && project.problems.length > 0 && (
        <Section title="Problems">
          <ul className="space-y-1">
            {project.problems.map((msg) => (
              <li key={msg} className="flex gap-1.5 text-sm text-amber-400">
                <AlertTriangle size={14} className="mt-0.5 flex-shrink-0" /> {msg}
              </li>
            ))}
          </ul>
        </Section>
      )}
      <HttpsSection project={project} />
      <Section title="Apps">
        {apps.length === 0 ? (
          <p className="text-sm text-[var(--muted-foreground)]">No apps configured yet.</p>
        ) : (
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-[var(--muted-foreground)]">
              <tr>
                <th className="py-1 font-medium">App</th>
                <th className="py-1 font-medium">Runtime</th>
                <th className="py-1 font-medium">Folder</th>
                <th className="py-1 font-medium">Address</th>
              </tr>
            </thead>
            <tbody>
              {apps.map((a) => (
                <tr key={a.name} className="border-t border-[var(--border)]">
                  <td className="py-2 font-medium text-[var(--foreground)]">{a.name}</td>
                  <td className="py-2 text-[var(--muted-foreground)]">
                    {a.runtime}
                    {a.version ? ` ${a.version}` : ""}
                    {a.service ? ` · ${a.service}` : ""}
                  </td>
                  <td className="py-2 text-[var(--muted-foreground)] font-mono text-xs">{a.path || "."}</td>
                  <td className="py-2">
                    {isUp ? (
                      <a href={a.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-blue-400 hover:underline">
                        {a.url.replace(/^https?:\/\//, "")} <ExternalLink size={12} />
                      </a>
                    ) : (
                      <span className="text-[var(--muted-foreground)]">{a.url.replace(/^https?:\/\//, "")}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>
      <Section title="Containers">
        {services.length === 0 ? (
          <p className="text-sm text-[var(--muted-foreground)]">No containers. Start the project to create them.</p>
        ) : (
          <ul className="divide-y divide-[var(--border)]">
            {services.map((s) => (
              <li key={s.name} className="flex items-center justify-between py-2 text-sm">
                <span className="font-mono text-xs text-[var(--foreground)]">{s.name}</span>
                <span className="flex items-center gap-2 text-xs text-[var(--muted-foreground)]">
                  {s.app ? `${s.app} · ` : ""}
                  {s.service}
                  <span
                    className={cn(
                      "px-2 py-0.5 rounded-full border",
                      s.state === "running"
                        ? "bg-green-500/15 text-green-400 border-green-500/30"
                        : "bg-slate-500/15 text-slate-300 border-slate-500/30",
                    )}
                  >
                    {s.state}
                  </span>
                  <span className="flex items-center">
                    {s.state === "running" ? (
                      <>
                        <IconAction label={`Restart ${s.name}`} disabled={pending !== null} onClick={() => containerAction(s.name, "restart")}>
                          <RotateCw size={13} className={pending === s.name ? "animate-spin" : undefined} />
                        </IconAction>
                        <IconAction label={`Stop ${s.name}`} disabled={pending !== null} onClick={() => containerAction(s.name, "stop")}>
                          <Square size={13} />
                        </IconAction>
                      </>
                    ) : (
                      <IconAction label={`Start ${s.name}`} disabled={pending !== null} onClick={() => containerAction(s.name, "start")}>
                        <Play size={13} className={pending === s.name ? "animate-pulse" : undefined} />
                      </IconAction>
                    )}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </Section>
    </div>
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

function HttpsSection({ project }: { project: ProjectView }) {
  const { setHttps, pending } = useSetHttps();
  const isUp = project.status === "running" || project.status === "partial";
  return (
    <Section
      title="HTTPS"
      actions={<HttpsSwitch project={project} pending={pending.has(project.id)} onChange={(on) => setHttps(project, on)} />}
    >
      <div className="space-y-2">
        <p className="text-sm text-[var(--muted-foreground)]">
          {!project.configured
            ? "Set the project up to turn HTTPS on."
            : project.tls
              ? isUp
                ? "Served over HTTPS with a certificate from SLD's certificate authority. Plain HTTP keeps working."
                : "HTTPS is on and applies when the project starts."
              : isUp
                ? "Served over plain HTTP. Turning HTTPS on applies it right away (containers may be recreated)."
                : "Served over plain HTTP."}
        </p>
        {project.tls && <CaTrustNotice compact />}
      </div>
    </Section>
  );
}

function Section({ title, children, actions }: { title: string; children: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <section className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-4">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-[var(--foreground)]">{title}</h2>
        {actions}
      </div>
      {children}
    </section>
  );
}

// ---------------------------------------------------------------- config

function useEditorTheme() {
  const { isDark } = useTheme();
  return isDark ? "vs-dark" : "light";
}

function ConfigEditor({
  project,
  config,
  onSaved,
  onRestart,
  onSetup,
}: {
  project: ProjectView;
  config?: string;
  onSaved: () => void;
  onRestart?: () => void;
  onSetup: () => void;
}) {
  const theme = useEditorTheme();
  const { success } = useToast();
  const [text, setText] = useState(config ?? "");
  const [saving, setSaving] = useState(false);
  const [problem, setProblem] = useState("");
  const [savedOnce, setSavedOnce] = useState(false);
  const dirty = text !== (config ?? "");

  // Take the server's copy when it changes and there are no local edits.
  const [base, setBase] = useState(config);
  if (config !== base) {
    setBase(config);
    if (!dirty) setText(config ?? "");
  }

  const save = async () => {
    setSaving(true);
    setProblem("");
    try {
      await v2.saveConfig(project.id, text);
      setSavedOnce(true);
      success("Config saved", onRestart ? "Restart the project to apply it." : undefined);
      onSaved();
    } catch (e) {
      setProblem((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  if (config === undefined && !dirty) {
    return (
      <Section title=".sld/config.yaml">
        <p className="text-sm text-[var(--muted-foreground)] mb-3">
          This project has no SLD config yet. Set it up to detect its stack and write one, then edit it here.
        </p>
        {project.status !== "undetected" ? (
          <Button size="sm" onClick={onSetup}>
            <Settings2 size={14} /> Set up
          </Button>
        ) : (
          <Button size="sm" variant="secondary" onClick={() => setText("version: 2\napps:\n  web:\n    runtime: static\n")}>
            <FilePlus size={14} /> Write one by hand
          </Button>
        )}
      </Section>
    );
  }

  return (
    <Section
      title=".sld/config.yaml"
      actions={
        <div className="flex items-center gap-1.5">
          {dirty && <span className="text-xs text-amber-400">Unsaved changes</span>}
          <Button size="sm" variant="ghost" disabled={!dirty || saving} onClick={() => { setText(config ?? ""); setProblem(""); }}>
            Discard
          </Button>
          <Button size="sm" onClick={save} disabled={!dirty} loading={saving}>
            <Save size={14} /> Save
          </Button>
        </div>
      }
    >
      {problem && (
        <pre className="mb-3 whitespace-pre-wrap rounded-md border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">{problem}</pre>
      )}
      {savedOnce && !dirty && onRestart && (
        <div className="mb-3 flex items-center justify-between rounded-md border border-blue-500/30 bg-blue-500/10 px-3 py-2 text-xs text-blue-300">
          Saved. The running containers still use the previous config.
          <Button size="sm" variant="secondary" onClick={onRestart}>
            <RotateCw size={14} /> Restart now
          </Button>
        </div>
      )}
      <div className="h-[480px] overflow-hidden rounded-md border border-[var(--border)]">
        <Editor
          height="100%"
          language="yaml"
          theme={theme}
          value={text}
          onChange={(v) => setText(v ?? "")}
          options={{ minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false, tabSize: 2 }}
        />
      </div>
      <p className="mt-2 text-xs text-[var(--muted-foreground)]">
        Checked before saving: typos in keys, unknown runtimes and invalid ports are refused with the line they are on.
      </p>
    </Section>
  );
}

// ---------------------------------------------------------------- files

function languageFor(name: string): string {
  const lower = name.toLowerCase();
  if (lower.startsWith(".env")) return "ini";
  if (lower === "dockerfile" || lower.endsWith(".dockerfile")) return "dockerfile";
  const ext = lower.split(".").pop() ?? "";
  const map: Record<string, string> = {
    php: "php", js: "javascript", jsx: "javascript", mjs: "javascript", cjs: "javascript",
    ts: "typescript", tsx: "typescript", json: "json", css: "css", scss: "scss", less: "less",
    html: "html", htm: "html", vue: "html", svelte: "html", md: "markdown", sql: "sql",
    yml: "yaml", yaml: "yaml", py: "python", rb: "ruby", go: "go", rs: "rust", java: "java",
    sh: "shell", bash: "shell", xml: "xml", ini: "ini", toml: "ini", conf: "ini",
  };
  return map[ext] ?? "plaintext";
}

function formatSize(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function FileBrowser({ projectId, onConfigSaved }: { projectId: string; onConfigSaved: () => void }) {
  const theme = useEditorTheme();
  const { success, error: toastError } = useToast();
  const [dir, setDir] = useState("");
  const [open, setOpen] = useState<FileContent | null>(null);
  const [openError, setOpenError] = useState("");
  const [text, setText] = useState("");
  const [saving, setSaving] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [newName, setNewName] = useState<string | null>(null);
  const dirty = open !== null && text !== open.content;

  const listing = useQuery({
    queryKey: ["v2", "files", projectId, dir],
    queryFn: () => v2.files(projectId, dir),
  });

  const openFile = async (path: string) => {
    if (dirty && !confirmDiscard()) return;
    setOpenError("");
    setConflict(false);
    try {
      const f = await v2.readFile(projectId, path);
      setOpen(f);
      setText(f.content);
    } catch (e) {
      setOpen(null);
      setOpenError(`${path}: ${(e as Error).message}`);
    }
  };

  const save = useCallback(
    async (force = false) => {
      if (!open) return;
      setSaving(true);
      try {
        const res = await v2.writeFile(projectId, open.path, text, force ? undefined : open.mod_time);
        setOpen({ ...open, content: text, mod_time: res.mod_time ?? open.mod_time });
        setConflict(false);
        success("Saved", open.path);
        if (open.path === ".sld/config.yaml") onConfigSaved();
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) setConflict(true);
        else toastError("Could not save", (e as Error).message);
      } finally {
        setSaving(false);
      }
    },
    [open, projectId, text, success, toastError, onConfigSaved],
  );

  const createFile = async () => {
    const name = (newName ?? "").trim();
    if (!name) return;
    const path = dir ? `${dir}/${name}` : name;
    try {
      await v2.writeFile(projectId, path, "");
      setNewName(null);
      await listing.refetch();
      openFile(path);
    } catch (e) {
      toastError("Could not create the file", (e as Error).message);
    }
  };

  const crumbs = dir ? dir.split("/") : [];

  return (
    <div className="flex h-[620px] overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--card)]">
      <div className="flex w-72 flex-shrink-0 flex-col border-r border-[var(--border)]">
        <div className="flex items-center gap-1 border-b border-[var(--border)] px-3 py-2 text-xs">
          <button className="text-blue-400 hover:underline" onClick={() => setDir("")}>
            root
          </button>
          {crumbs.map((c, i) => (
            <span key={i} className="flex items-center gap-1 min-w-0">
              <ChevronRight size={12} className="text-[var(--muted-foreground)]" />
              <button className="truncate text-blue-400 hover:underline" onClick={() => setDir(crumbs.slice(0, i + 1).join("/"))}>
                {c}
              </button>
            </span>
          ))}
          <span className="ml-auto flex gap-0.5">
            <button title="New file" className="p-1 rounded hover:bg-[var(--card-hover)]" onClick={() => setNewName("")}>
              <FilePlus size={14} />
            </button>
            <button title="Reload" className="p-1 rounded hover:bg-[var(--card-hover)]" onClick={() => listing.refetch()}>
              <RefreshCw size={14} className={cn(listing.isFetching && "animate-spin")} />
            </button>
          </span>
        </div>
        {newName !== null && (
          <form
            className="flex gap-1 border-b border-[var(--border)] p-2"
            onSubmit={(e) => {
              e.preventDefault();
              createFile();
            }}
          >
            <input
              autoFocus
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && setNewName(null)}
              placeholder="file name"
              className="h-7 min-w-0 flex-1 rounded border border-[var(--border)] bg-[var(--background)] px-2 text-xs text-[var(--foreground)]"
            />
            <Button size="sm" type="submit">
              Create
            </Button>
          </form>
        )}
        <div className="flex-1 overflow-y-auto py-1 text-sm">
          {listing.error && <p className="p-3 text-xs text-red-400">{(listing.error as Error).message}</p>}
          {listing.data?.entries.map((e) => (
            <button
              key={e.path}
              onClick={() => (e.is_dir ? setDir(e.path) : openFile(e.path))}
              className={cn(
                "flex w-full items-center gap-2 px-3 py-1 text-left hover:bg-[var(--card-hover)]",
                open?.path === e.path && "bg-[var(--card-hover)] text-[var(--foreground)]",
              )}
            >
              {e.is_dir ? (
                <Folder size={14} className="flex-shrink-0 text-blue-400" />
              ) : (
                <File size={14} className="flex-shrink-0 text-[var(--muted-foreground)]" />
              )}
              <span className="truncate text-[var(--foreground)]">{e.name}</span>
              {!e.is_dir && <span className="ml-auto text-[10px] text-[var(--muted-foreground)]">{formatSize(e.size)}</span>}
            </button>
          ))}
          {listing.data && listing.data.entries.length === 0 && (
            <p className="p-3 text-xs text-[var(--muted-foreground)]">Empty folder.</p>
          )}
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        {open ? (
          <>
            <div className="flex items-center justify-between gap-2 border-b border-[var(--border)] px-3 py-2">
              <span className="truncate font-mono text-xs text-[var(--foreground)]">
                {open.path}
                {dirty && <span className="ml-2 text-amber-400">●</span>}
              </span>
              <div className="flex gap-1">
                <Button size="sm" variant="ghost" disabled={!dirty} onClick={() => setText(open.content)}>
                  Discard
                </Button>
                <Button size="sm" onClick={() => save()} disabled={!dirty} loading={saving}>
                  <Save size={14} /> Save
                </Button>
              </div>
            </div>
            {conflict && (
              <div className="flex items-center justify-between gap-2 border-b border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-300">
                This file changed on disk after you opened it.
                <span className="flex gap-1">
                  <Button size="sm" variant="secondary" onClick={() => openFile(open.path)}>
                    Reload (lose my edits)
                  </Button>
                  <Button size="sm" variant="danger" onClick={() => save(true)}>
                    Overwrite
                  </Button>
                </span>
              </div>
            )}
            <div
              className="flex-1"
              onKeyDown={(e) => {
                if ((e.ctrlKey || e.metaKey) && e.key === "s") {
                  e.preventDefault();
                  if (dirty) save();
                }
              }}
            >
              <Editor
                height="100%"
                theme={theme}
                path={open.path}
                language={languageFor(open.path.split("/").pop() ?? "")}
                value={text}
                onChange={(v) => setText(v ?? "")}
                options={{ minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false }}
              />
            </div>
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center p-6 text-center text-sm text-[var(--muted-foreground)]">
            {openError ? <span className="text-red-400">{openError}</span> : "Choose a file to view or edit it."}
          </div>
        )}
      </div>
    </div>
  );
}

function confirmDiscard() {
  // A native confirm is acceptable here: it only guards against losing typed edits.
  return window.confirm("Discard your unsaved changes?");
}

// ---------------------------------------------------------------- env

const SECRET_KEY = /(SECRET|PASSWORD|PASS|TOKEN|KEY|PRIVATE|CREDENTIAL)/i;

interface Row extends EnvVar {
  rid: number;
}

let nextRid = 1;
const toRows = (vars: EnvVar[]): Row[] => vars.map((v) => ({ ...v, rid: nextRid++ }));

function EnvEditor({ projectId, onRestart }: { projectId: string; onRestart?: () => void }) {
  const { success, error: toastError } = useToast();
  const queryClient = useQueryClient();
  const envQuery = useQuery({ queryKey: ["v2", "env", projectId], queryFn: () => v2.env(projectId) });
  const files = useMemo(() => envQuery.data?.files ?? [], [envQuery.data]);
  const known = envQuery.data?.known ?? [];

  const [name, setName] = useState<string>("");
  const [rows, setRows] = useState<Row[]>([]);
  const [revealed, setRevealed] = useState<Set<number>>(new Set());
  const [saving, setSaving] = useState(false);
  const [showBackups, setShowBackups] = useState(false);
  const [filter, setFilter] = useState("");

  const current = files.find((f) => f.name === name);
  const original = useMemo(() => current?.variables ?? [], [current]);
  const dirty = JSON.stringify(rows.map(({ key, value }) => ({ key, value }))) !== JSON.stringify(original);

  // Pick the first file once loaded, and reload rows when the file changes on the server.
  useEffect(() => {
    if (!name && files.length > 0) setName(files[0].name);
  }, [files, name]);
  useEffect(() => {
    setRows(toRows(original));
    setRevealed(new Set());
  }, [original]);

  const missing = known.filter((k) => !files.some((f) => f.name === k));
  const example = files.find((f) => f.name === ".env.example");

  const save = async () => {
    const keys = rows.map((r) => r.key.trim());
    const dup = keys.find((k, i) => k && keys.indexOf(k) !== i);
    if (dup) {
      toastError("Duplicate variable", dup);
      return;
    }
    setSaving(true);
    try {
      await v2.saveEnv(
        projectId,
        name,
        rows.filter((r) => r.key.trim()).map((r) => ({ key: r.key.trim(), value: r.value })),
      );
      success(`${name} saved`, onRestart ? "Restart the project so the app picks it up." : undefined);
      await queryClient.invalidateQueries({ queryKey: ["v2", "env", projectId] });
    } catch (e) {
      toastError("Could not save", (e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const createFile = async (file: string) => {
    try {
      const seed = file === ".env" && example ? example.variables : [];
      await v2.saveEnv(projectId, file, seed);
      await queryClient.invalidateQueries({ queryKey: ["v2", "env", projectId] });
      setName(file);
    } catch (e) {
      toastError("Could not create the file", (e as Error).message);
    }
  };

  if (envQuery.isLoading) return <p className="text-sm text-[var(--muted-foreground)]">Reading env files…</p>;
  if (envQuery.error) return <p className="text-sm text-red-400">{(envQuery.error as Error).message}</p>;

  const q = filter.trim().toLowerCase();
  const shown = rows.filter((r) => !q || r.key.toLowerCase().includes(q));

  return (
    <Section
      title="Environment files"
      actions={
        <div className="flex flex-wrap items-center gap-1.5">
          {missing.length > 0 && (
            <select
              value=""
              onChange={(e) => e.target.value && createFile(e.target.value)}
              className="h-8 rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-xs text-[var(--foreground)]"
              aria-label="Create env file"
            >
              <option value="">New file…</option>
              {missing.map((m) => (
                <option key={m} value={m}>
                  {m}
                  {m === ".env" && example ? " (from .env.example)" : ""}
                </option>
              ))}
            </select>
          )}
          {current && (
            <Button size="sm" variant="ghost" onClick={() => setShowBackups((s) => !s)}>
              <History size={14} /> Backups
            </Button>
          )}
        </div>
      }
    >
      {files.length === 0 ? (
        <p className="text-sm text-[var(--muted-foreground)]">No .env files in this project. Create one with “New file…”.</p>
      ) : (
        <>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            {files.map((f) => (
              <button
                key={f.name}
                onClick={() => {
                  if (dirty && f.name !== name && !confirmDiscard()) return;
                  setName(f.name);
                  setShowBackups(false);
                }}
                className={cn(
                  "h-7 rounded-full border px-3 font-mono text-xs",
                  f.name === name
                    ? "border-transparent bg-[var(--primary)] text-[var(--primary-foreground)]"
                    : "border-[var(--border)] text-[var(--muted-foreground)] hover:text-[var(--foreground)]",
                )}
              >
                {f.name}
              </button>
            ))}
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter variables"
              className="ml-auto h-8 w-48 rounded-lg border border-[var(--border)] bg-[var(--background)] px-3 text-xs text-[var(--foreground)]"
            />
          </div>

          {showBackups && current && (
            <EnvBackups
              projectId={projectId}
              name={current.name}
              onRestored={() => {
                setShowBackups(false);
                queryClient.invalidateQueries({ queryKey: ["v2", "env", projectId] });
              }}
            />
          )}

          <div className="space-y-1.5">
            {shown.map((r) => {
              const secret = SECRET_KEY.test(r.key);
              const visible = !secret || revealed.has(r.rid);
              return (
                <div key={r.rid} className="flex items-center gap-1.5">
                  <input
                    value={r.key}
                    onChange={(e) => setRows((rs) => rs.map((x) => (x.rid === r.rid ? { ...x, key: e.target.value } : x)))}
                    placeholder="NAME"
                    spellCheck={false}
                    className="h-8 w-64 rounded-md border border-[var(--border)] bg-[var(--background)] px-2 font-mono text-xs text-[var(--foreground)]"
                  />
                  <input
                    value={r.value}
                    type={visible ? "text" : "password"}
                    onChange={(e) => setRows((rs) => rs.map((x) => (x.rid === r.rid ? { ...x, value: e.target.value } : x)))}
                    placeholder="value"
                    spellCheck={false}
                    autoComplete="off"
                    className="h-8 min-w-0 flex-1 rounded-md border border-[var(--border)] bg-[var(--background)] px-2 font-mono text-xs text-[var(--foreground)]"
                  />
                  {secret && (
                    <button
                      title={visible ? "Hide" : "Show"}
                      className="p-1.5 rounded text-[var(--muted-foreground)] hover:bg-[var(--card-hover)]"
                      onClick={() =>
                        setRevealed((s) => {
                          const n = new Set(s);
                          if (n.has(r.rid)) n.delete(r.rid);
                          else n.add(r.rid);
                          return n;
                        })
                      }
                    >
                      {visible ? <EyeOff size={14} /> : <Eye size={14} />}
                    </button>
                  )}
                  <button
                    title="Remove"
                    className="p-1.5 rounded text-[var(--muted-foreground)] hover:bg-[var(--card-hover)] hover:text-red-400"
                    onClick={() => setRows((rs) => rs.filter((x) => x.rid !== r.rid))}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              );
            })}
          </div>

          <div className="mt-3 flex items-center justify-between gap-2">
            <Button size="sm" variant="ghost" onClick={() => setRows((rs) => [...rs, { key: "", value: "", rid: nextRid++ }])}>
              <Plus size={14} /> Add variable
            </Button>
            <div className="flex items-center gap-1.5">
              {dirty && <span className="text-xs text-amber-400">Unsaved changes</span>}
              <Button size="sm" variant="ghost" disabled={!dirty} onClick={() => setRows(toRows(original))}>
                Discard
              </Button>
              <Button size="sm" onClick={save} disabled={!dirty} loading={saving}>
                <Save size={14} /> Save
              </Button>
            </div>
          </div>
          <p className="mt-2 text-xs text-[var(--muted-foreground)]">
            Comments and the order of the file are kept. A backup of the previous version is stored in SLD (not in your
            project) on every save.
          </p>
        </>
      )}
    </Section>
  );
}

function EnvBackups({ projectId, name, onRestored }: { projectId: string; name: string; onRestored: () => void }) {
  const { success, error: toastError } = useToast();
  const backups = useQuery({ queryKey: ["v2", "env-backups", projectId, name], queryFn: () => v2.envBackups(projectId, name) });
  return (
    <div className="mb-3 rounded-md border border-[var(--border)] p-3">
      {backups.isLoading && <p className="text-xs text-[var(--muted-foreground)]">Loading backups…</p>}
      {backups.data?.length === 0 && <p className="text-xs text-[var(--muted-foreground)]">No backups of {name} yet.</p>}
      <ul className="divide-y divide-[var(--border)]">
        {backups.data?.map((b) => (
          <li key={b.id} className="flex items-center justify-between py-1.5 text-xs">
            <span className="text-[var(--foreground)]">{new Date(b.created_at).toLocaleString()}</span>
            <span className="flex items-center gap-2 text-[var(--muted-foreground)]">
              {formatSize(b.size)}
              <Button
                size="sm"
                variant="secondary"
                onClick={async () => {
                  try {
                    await v2.restoreEnv(projectId, b.id);
                    success(`${name} restored`);
                    onRestored();
                  } catch (e) {
                    toastError("Could not restore", (e as Error).message);
                  }
                }}
              >
                Restore
              </Button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
