/**
 * Client for SLD's control-plane API (/api/v2): projects, jobs, logs, system.
 */

export type ProjectStatus = "running" | "partial" | "stopped" | "not-configured" | "undetected" | "invalid";
export type ProjectAction = "setup" | "start" | "stop" | "restart" | "destroy";
export type JobStatus = "running" | "succeeded" | "failed";

export interface AppView {
  name: string;
  runtime: string;
  version?: string;
  domain: string;
  path?: string;
  service?: string;
  url: string;
}

export interface ServiceRef {
  name: string;
  service: string;
  app?: string;
  state: string;
}

export interface ProjectView {
  id: string;
  name: string;
  folder: string;
  status: ProjectStatus;
  configured: boolean;
  apps: AppView[] | null;
  containers: number;
  running: number;
  problems?: string[];
  job_id?: string;
  autostart?: boolean;
  services?: ServiceRef[];
  /** Has its own compose file or Dockerfile, or SLD already runs containers for it. */
  dockerized: boolean;
  /** The parked/linked folder that makes SLD detect it (relative to its workspace), if any. */
  parked_in?: string;
  /** Served over HTTPS (domain.tls in its config). */
  tls: boolean;
}

/** A workspace project SLD does not detect yet (no Docker files, not parked or linked). */
export interface Candidate {
  path: string;
  folder: string;
  name: string;
  apps: Omit<AppView, "url">[];
  /** SLD recognised the stack and can write the config itself. */
  detected: boolean;
  /** Already has a .sld/config.yaml. */
  configured: boolean;
  problems?: string[];
  /** Why it cannot be dockerized on its own (WordPress plugin, Apps Script add-on...). */
  reason?: string;
}

export interface AutomationSettings {
  auto_setup: boolean;
  restore_running: boolean;
  autostart_all: boolean;
}

export interface JobView {
  id: string;
  kind: string;
  project_id?: string;
  status: JobStatus;
  error?: string;
  started_at: string;
  ended_at?: string;
  output: string;
  offset: number;
}

export interface SystemInfo {
  version: string;
  in_container: boolean;
  data_dir: string;
  bind_host: string;
  http_port: number;
  https_port: number;
  tld: string;
  workspaces: string[] | null;
  docker: boolean;
  router: string; // running | exited | missing | ...
  ca_available?: boolean;
  mounts?: { type: string; destination: string; source: string; volume: string }[] | null;
}

export interface FileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export interface FileContent {
  path: string;
  content: string;
  size: number;
  mod_time: string;
}

export interface EnvVar {
  key: string;
  value: string;
}

export interface EnvFileView {
  name: string;
  variables: EnvVar[];
  mod_time: string;
}

export interface TaskView {
  name: string;
  command: string;
  app?: string;
  description?: string;
  /** "config": from the project's tasks: block; "suggested": found by SLD. */
  source: "config" | "suggested";
}

export type ContainerAction = "start" | "stop" | "restart";

export interface EnvBackup {
  id: string;
  name: string;
  created_at: string;
  size: number;
}

export class ApiError extends Error {
  status: number;
  jobId?: string;
  constructor(message: string, status: number, jobId?: string) {
    super(message);
    this.status = status;
    this.jobId = jobId;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api/v2${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
  });
  if (!res.ok) {
    let message = res.statusText;
    let jobId: string | undefined;
    try {
      const body = await res.json();
      message = body.error || message;
      jobId = body.job_id;
    } catch {
      /* not JSON */
    }
    throw new ApiError(message, res.status, jobId);
  }
  return res.json();
}

export const v2 = {
  authStatus: () => request<{ authenticated: boolean; token_command?: string }>("/auth/status"),
  login: (token: string) =>
    request<{ authenticated: boolean }>("/auth/login", { method: "POST", body: JSON.stringify({ token }) }),
  logout: () => request<{ authenticated: boolean }>("/auth/logout", { method: "POST" }),

  system: () => request<SystemInfo>("/system"),
  initRouter: () => request<JobView>("/system/init", { method: "POST" }),
  caCertUrl: "/api/v2/system/ca.crt",

  projects: (refresh = false) => request<ProjectView[]>(`/projects${refresh ? "?refresh=1" : ""}`),
  project: (id: string) => request<{ project: ProjectView; config?: string }>(`/projects/${encodeURIComponent(id)}`),
  action: (id: string, action: ProjectAction, body?: { remove_volumes?: boolean }) =>
    request<JobView>(`/projects/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      body: JSON.stringify(body || {}),
    }),
  job: (id: string) => request<JobView>(`/jobs/${encodeURIComponent(id)}`),

  settings: () => request<AutomationSettings>("/settings"),
  updateSettings: (patch: Partial<AutomationSettings>) =>
    request<AutomationSettings>("/settings", { method: "PUT", body: JSON.stringify(patch) }),
  setAutostart: (id: string, enabled: boolean) =>
    request<{ id: string; autostart: boolean }>(`/projects/${encodeURIComponent(id)}/autostart`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),

  /** Turns HTTPS on or off; a running project gets a job that applies it. */
  setTLS: (id: string, enabled: boolean) =>
    request<{ tls: boolean; job?: JobView }>(`/projects/${encodeURIComponent(id)}/tls`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),

  /** Workspace projects that are not dockerized, parked or linked yet. */
  candidates: () => request<Candidate[]>("/candidates"),
  /**
   * Generates Docker files for the chosen projects (each with the sub-folder apps to include;
   * no apps = all) and optionally builds and starts them, in one job.
   */
  dockerize: (projects: { path: string; apps?: string[] }[], start: boolean) =>
    request<JobView>("/dockerize", { method: "POST", body: JSON.stringify({ projects, start }) }),

  // Parked and linked folders: projects in them are detected even without Docker files.
  parked: () => request<{ path: string; folder: string }[]>("/parked"),
  removeParked: (path: string) =>
    request<{ removed: string }>("/parked/remove", { method: "POST", body: JSON.stringify({ path }) }),
  setLinked: (id: string, linked: boolean) =>
    request<{ linked: boolean; removed?: string }>(`/projects/${encodeURIComponent(id)}/link`, {
      method: "POST",
      body: JSON.stringify({ linked }),
    }),
  /** Parks a folder (a project, or a folder of projects), given as a path inside the SLD container. */
  async park(path: string): Promise<string> {
    const res = await fetch("/api/park", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new ApiError(body.error || res.statusText, res.status);
    return body.message || "";
  },
  async directories(path: string): Promise<string[]> {
    const res = await fetch(`/api/system/directories?path=${encodeURIComponent(path)}`);
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new ApiError(body.error || res.statusText, res.status);
    return body as string[];
  },

  // Project detail: config, files and env. Paths are relative to the project folder.
  saveConfig: (id: string, config: string) =>
    request<{ saved: boolean }>(`/projects/${encodeURIComponent(id)}/config`, {
      method: "PUT",
      body: JSON.stringify({ config }),
    }),
  files: (id: string, path = "") =>
    request<{ path: string; entries: FileEntry[] }>(
      `/projects/${encodeURIComponent(id)}/files?path=${encodeURIComponent(path)}`,
    ),
  readFile: (id: string, path: string) =>
    request<FileContent>(`/projects/${encodeURIComponent(id)}/file?path=${encodeURIComponent(path)}`),
  writeFile: (id: string, path: string, content: string, modTime?: string) =>
    request<{ path: string; mod_time?: string }>(`/projects/${encodeURIComponent(id)}/file`, {
      method: "PUT",
      body: JSON.stringify({ path, content, mod_time: modTime }),
    }),
  env: (id: string) => request<{ files: EnvFileView[]; known: string[] }>(`/projects/${encodeURIComponent(id)}/env`),
  saveEnv: (id: string, name: string, variables: EnvVar[]) =>
    request<{ saved: boolean }>(`/projects/${encodeURIComponent(id)}/env`, {
      method: "PUT",
      body: JSON.stringify({ name, variables }),
    }),
  envBackups: (id: string, name: string) =>
    request<EnvBackup[]>(`/projects/${encodeURIComponent(id)}/env/backups?name=${encodeURIComponent(name)}`),
  restoreEnv: (id: string, backupId: string) =>
    request<{ restored: string }>(`/projects/${encodeURIComponent(id)}/env/restore`, {
      method: "POST",
      body: JSON.stringify({ id: backupId }),
    }),

  // Tasks and commands run inside a project's app containers (as background jobs).
  tasks: (id: string) =>
    request<{ tasks: TaskView[]; apps: string[]; error?: string }>(`/projects/${encodeURIComponent(id)}/tasks`),
  runTask: (id: string, task: string, app?: string) =>
    request<JobView>(`/projects/${encodeURIComponent(id)}/exec`, {
      method: "POST",
      body: JSON.stringify({ task, app }),
    }),
  runCommand: (id: string, command: string, app?: string) =>
    request<JobView>(`/projects/${encodeURIComponent(id)}/exec`, {
      method: "POST",
      body: JSON.stringify({ command, app }),
    }),
  /** Starts, stops or restarts one container of a project. */
  containerAction: (id: string, container: string, action: ContainerAction) =>
    request<{ ok: boolean }>(
      `/projects/${encodeURIComponent(id)}/containers/${encodeURIComponent(container)}/${action}`,
      { method: "POST" },
    ),

  /** Streams a job's output; returns a function that stops listening. */
  followJob(id: string, onOutput: (text: string) => void, onDone: (job: JobView) => void): () => void {
    const source = new EventSource(`/api/v2/jobs/${encodeURIComponent(id)}/events`);
    source.addEventListener("output", (e) => onOutput(JSON.parse((e as MessageEvent).data)));
    source.addEventListener("done", (e) => {
      onDone(JSON.parse((e as MessageEvent).data));
      source.close();
    });
    source.onerror = () => {
      // The stream ends after "done"; on other errors fall back to one fetch.
      if (source.readyState === EventSource.CLOSED) return;
      source.close();
      v2.job(id).then((job) => {
        if (job.output) onOutput(job.output);
        if (job.status !== "running") onDone(job);
      }).catch(() => undefined);
    };
    return () => source.close();
  },

  /** Fetches (and with follow, streams) logs of a project container. */
  async logs(
    id: string,
    opts: { target?: string; tail?: number; follow?: boolean; signal?: AbortSignal },
    onChunk: (text: string) => void,
  ): Promise<string | null> {
    const params = new URLSearchParams();
    if (opts.target) params.set("target", opts.target);
    params.set("tail", String(opts.tail ?? 300));
    if (opts.follow) params.set("follow", "1");
    const res = await fetch(`/api/v2/projects/${encodeURIComponent(id)}/logs?${params}`, { signal: opts.signal });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new ApiError(body.error || res.statusText, res.status);
    }
    const container = res.headers.get("X-SLD-Container");
    const reader = res.body?.getReader();
    if (!reader) return container;
    const decoder = new TextDecoder();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      onChunk(decoder.decode(value, { stream: true }));
    }
    return container;
  },
};
