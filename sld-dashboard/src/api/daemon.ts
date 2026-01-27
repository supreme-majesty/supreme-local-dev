/**
 * SLD Daemon API Client
 * Communicates with the Go daemon running on port 2025
 */

const API_BASE = "/api";

// Types matching Go daemon state
export interface SLDState {
  tld: string;
  paths: string[];
  links: Record<string, string>;
  services: Record<string, string>;
  certificates: string[];
  php_version: string;
  secure: boolean;
  port: string;
}

export interface ActionResponse {
  success: boolean;
  message?: string;
  error?: string;
}

export interface ServiceStatus {
  name: string;
  running: boolean;
  version?: string;
}

export interface Project {
  name: string;
  path: string;
  domain: string;
  phpVersion?: string;
  secure: boolean;
  type: "parked" | "linked";
  creating?: boolean; // true if project is still being created in background
  tags?: string[];
  category?: string;
}

export interface HealthCheck {
  name: string;
  status: "pass" | "fail" | "warn";
  message: string;
  fixable?: boolean;
}

// Supreme Healer Types
export interface HealerIssue {
  id: string;
  title: string;
  description: string;
  severity: "info" | "warning" | "critical";
  source: string;
  detected_at: string;
  fix_action: string;
  can_auto_fix: boolean;
}

export interface Plugin {
  id: string;
  name: string;
  description: string;
  version: string;
  installed: boolean;
  status: "running" | "stopped" | "installing" | "not_installed";
}

export interface DatabaseInfo {
  name: string;
  tables: number;
}

export interface TableInfo {
  name: string;
  row_count: number;
  engine: string;
  collation: string;
  size: number;
  overhead: number;
}

export interface ColumnInfo {
  name: string;
  type: string;
  nullable: boolean;
  key: string;
  default: string;
  foreign_key?: {
    table: string;
    column: string;
  };
}

export interface TableData {
  columns: ColumnInfo[];
  rows: Record<string, any>[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
  query_time?: number; // Query execution time in seconds (when profiling enabled)
}

export interface Snapshot {
  id: string;
  database: string;
  table?: string;
  filename: string;
  size: number;
  created_at: string;
}

export interface QueryResult {
  columns: string[] | null;
  rows: any[] | null;
  rowCount: number;
  message?: string;
  error?: string;
}

export interface Editor {
  id: string;
  name: string;
  bin: string;
  icon: string;
}

export interface ProjectTemplate {
  id: string;
  name: string;
  description: string;
  icon: string;
}

export interface ProjectOptions {
  type: string;
  name: string;
  directory?: string;
  repository?: string;
}

export interface Tunnel {
  site_name: string;
  public_url: string;
  started_at: string;
}

// Env Manager types
export interface EnvFile {
  path: string;
  name: string;
  variables: Record<string, string>;
  mod_time: string;
}

export interface EnvBackup {
  filename: string;
  path: string;
  created_at: string;
  size: number;
}

// API Client
class DaemonApi {
  private async request<T>(
    endpoint: string,
    options?: RequestInit,
  ): Promise<T> {
    const url = `${API_BASE}${endpoint}${
      endpoint.includes("?") ? "&" : "?"
    }t=${Date.now()}`;
    let response;
    try {
      response = await fetch(url, {
        headers: {
          "Content-Type": "application/json",
        },
        ...options,
      });
    } catch (error) {
      console.error("Daemon API Request Error:", error);
      throw new Error(
        "Unable to connect to SLD Daemon. Please ensure the service is running via 'sld service start' or './sld daemon'.",
      );
    }

    if (!response.ok) {
      let errorMessage: string;
      const contentType = response.headers.get("content-type");
      if (contentType && contentType.includes("application/json")) {
        const json = await response.json().catch(() => ({}));
        errorMessage = json.error || json.message || "Unknown error";
      } else {
        errorMessage = await response.text();
      }
      throw new Error(errorMessage || `Request failed: ${response.statusText}`);
    }

    return response.json();
  }

  // Database Management
  async getDatabases(): Promise<DatabaseInfo[]> {
    return this.request<DatabaseInfo[]>("/db/databases");
  }

  async createDatabase(name: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/db/create", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
  }

  async deleteDatabase(name: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/db/delete", {
      method: "DELETE",
      body: JSON.stringify({ name }),
    });
  }

  async getTables(database: string): Promise<TableInfo[]> {
    return this.request<TableInfo[]>(`/db/tables?db=${database}`);
  }

  async getTableData(
    database: string,
    table: string,
    page: number = 1,
    options: {
      perPage?: number;
      sortCol?: string;
      sortOrder?: "ASC" | "DESC";
      profile?: boolean;
    } = {},
  ): Promise<TableData> {
    const params = new URLSearchParams({
      db: database,
      table: table,
      limit: String(options.perPage || 50),
      offset: String((page - 1) * (options.perPage || 50)),
    });
    if (options.sortCol) params.set("sort", options.sortCol);
    if (options.sortOrder) params.set("order", options.sortOrder);
    if (options.profile) params.set("profile", "true");

    return this.request<TableData>(`/db/table?${params.toString()}`);
  }

  async getTableSchema(database: string, table: string): Promise<ColumnInfo[]> {
    return this.request<ColumnInfo[]>(
      `/db/schema?db=${database}&table=${table}`,
    );
  }

  async getForeignValues(
    database: string,
    table: string,
    column: string,
  ): Promise<{ value: string; label: string }[]> {
    const params = new URLSearchParams({ database, table, column });
    return this.request<{ value: string; label: string }[]>(
      `/db/foreign-values?${params.toString()}`,
    );
  }
  // Actually, wait, let's optimize. The backend `ExecuteQuery` can run `DESCRIBE`.
  // But let's keep it simple. Using `getTableData` is fine for now.

  async getSnapshots(): Promise<Snapshot[]> {
    return this.request<Snapshot[]>("/db/snapshots");
  }

  async createSnapshot(database: string, table?: string): Promise<Snapshot> {
    return this.request<Snapshot>("/db/snapshots", {
      method: "POST",
      body: JSON.stringify({ database, table }),
    });
  }

  async restoreSnapshot(filename: string): Promise<void> {
    return this.request("/db/restore", {
      method: "POST",
      body: JSON.stringify({ filename }),
    });
  }

  async deleteSnapshot(filename: string): Promise<void> {
    return this.request("/db/snapshots", {
      method: "DELETE",
      body: JSON.stringify({ filename }),
    });
  }

  // Time-Travel: Rewind database to a snapshot (creates auto-backup first)
  async rewindDatabase(
    filename: string,
  ): Promise<{ success: boolean; message: string; backup: Snapshot }> {
    return this.request("/db/rewind", {
      method: "POST",
      body: JSON.stringify({ filename }),
    });
  }

  async executeQuery(database: string, query: string): Promise<QueryResult> {
    return this.request<QueryResult>("/db/query", {
      method: "POST",
      body: JSON.stringify({ database, query }),
    });
  }

  async importDatabase(
    file: File,
    database: string,
    restore: boolean = true,
  ): Promise<void> {
    const formData = new FormData();
    formData.append("file", file);
    formData.append("database", database);

    const res = await fetch(`${API_BASE}/db/import?restore=${restore}`, {
      method: "POST",
      body: formData,
    });

    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || "Import failed");
    }
  }

  // State
  async getState(): Promise<SLDState> {
    return this.request<SLDState>("/state");
  }

  // Project Management
  async park(path: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/park", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
  }

  async forget(path: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/forget", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
  }

  async link(name: string, path: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/link", {
      method: "POST",
      body: JSON.stringify({ name, path }),
    });
  }

  async unlink(name: string): Promise<void> {
    return this.request("/unlink", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
  }

  async ignore(path: string): Promise<boolean> {
    const res = await this.request<ActionResponse>("/ignore", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
    return res.success;
  }

  async unignore(path: string): Promise<boolean> {
    const res = await this.request<ActionResponse>("/unignore", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
    return res.success;
  }

  async getPlugins(): Promise<Plugin[]> {
    const res = await this.request<Plugin[]>("/plugins");
    return res || [];
  }

  async installPlugin(id: string): Promise<boolean> {
    const res = await this.request<ActionResponse>("/plugins/install", {
      method: "POST",
      body: JSON.stringify({ id }),
    });
    return res.success;
  }

  async togglePlugin(id: string, enabled: boolean): Promise<boolean> {
    const res = await this.request<ActionResponse>("/plugins/toggle", {
      method: "POST",
      body: JSON.stringify({ id, enabled }),
    });
    return res.success;
  }

  // PHP Management
  async switchPHP(version: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/php", {
      method: "POST",
      body: JSON.stringify({ version }),
    });
  }

  async getPHPVersions(): Promise<string[]> {
    return this.request<string[]>("/php/versions");
  }

  // SSL/HTTPS
  async secure(): Promise<ActionResponse> {
    return this.request<ActionResponse>("/secure", {
      method: "POST",
    });
  }

  // Projects & System
  async createProject(options: ProjectOptions): Promise<ActionResponse> {
    return this.request<ActionResponse>("/projects/create", {
      method: "POST",
      body: JSON.stringify(options),
    });
  }

  async getTemplates(): Promise<ProjectTemplate[]> {
    return this.request<ProjectTemplate[]>("/projects/templates");
  }

  // Ghost Mode: Clone a project for experimentation
  async ghostProject(
    sourcePath: string,
    targetName?: string,
    cloneDB: boolean = true,
  ): Promise<ActionResponse> {
    return this.request<ActionResponse>("/projects/ghost", {
      method: "POST",
      body: JSON.stringify({
        source_path: sourcePath,
        target_name: targetName,
        clone_db: cloneDB,
      }),
    });
  }

  async getEditors(): Promise<Editor[]> {
    return this.request<Editor[]>("/system/editors");
  }

  async openInEditor(path: string, editor: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/system/open-editor", {
      method: "POST",
      body: JSON.stringify({ path, editor }),
    });
  }

  async getDirectories(path?: string): Promise<string[]> {
    const params = new URLSearchParams();
    if (path) params.set("path", path);
    return this.request<string[]>(`/system/directories?${params.toString()}`);
  }

  // Service Management
  async getServiceStatus(): Promise<ServiceStatus[]> {
    return this.request<ServiceStatus[]>("/services");
  }

  async controlService(
    service: string,
    action: "start" | "stop" | "restart",
  ): Promise<ActionResponse> {
    return this.request<ActionResponse>("/services/control", {
      method: "POST",
      body: JSON.stringify({ service, action }),
    });
  }

  async restart(): Promise<ActionResponse> {
    return this.request<ActionResponse>("/restart", {
      method: "POST",
    });
  }

  // Doctor/Diagnostics (to be implemented in daemon)
  // Doctor/Diagnostics
  async runDoctor(): Promise<HealthCheck[]> {
    return this.request<HealthCheck[]>("/system/doctor");
  }

  // Helper to transform state into projects list
  async getProjects(): Promise<Project[]> {
    const projects = await this.request<Project[]>("/sites");
    return projects || [];
  }

  async updateSite(
    domain: string,
    tags: string[],
    category: string,
  ): Promise<ActionResponse> {
    return this.request<ActionResponse>("/sites/update", {
      method: "POST",
      body: JSON.stringify({ domain, tags, category }),
    });
  }

  // Sharing / Tunnels
  async shareStart(site: string): Promise<string> {
    const res = await this.request<ActionResponse>("/share/start", {
      method: "POST",
      body: JSON.stringify({ site }),
    });
    return res.message || "";
  }

  async shareStop(site: string): Promise<void> {
    return this.request("/share/stop", {
      method: "POST",
      body: JSON.stringify({ site }),
    });
  }

  async getShareStatus(): Promise<Tunnel[]> {
    return this.request<Tunnel[]>("/share/status");
  }

  // ============ Phase 2 Features ============

  // Env Manager
  async getEnvFiles(projectPath: string): Promise<EnvFile[]> {
    return this.request<EnvFile[]>(
      `/env/files?project=${encodeURIComponent(projectPath)}`,
    );
  }

  async readEnvFile(path: string): Promise<EnvFile> {
    return this.request<EnvFile>(`/env/read?path=${encodeURIComponent(path)}`);
  }

  async writeEnvFile(
    path: string,
    variables: Record<string, string>,
  ): Promise<ActionResponse> {
    return this.request<ActionResponse>("/env/write", {
      method: "PUT",
      body: JSON.stringify({ path, variables }),
    });
  }

  async getEnvBackups(path: string): Promise<EnvBackup[]> {
    return this.request<EnvBackup[]>(
      `/env/backups?path=${encodeURIComponent(path)}`,
    );
  }

  async restoreEnvBackup(
    backupPath: string,
    targetPath: string,
  ): Promise<ActionResponse> {
    return this.request<ActionResponse>("/env/restore", {
      method: "POST",
      body: JSON.stringify({
        backup_path: backupPath,
        target_path: targetPath,
      }),
    });
  }

  // Artisan Runner
  async runArtisanCommand(
    projectPath: string,
    command: string,
  ): Promise<ActionResponse> {
    return this.request<ActionResponse>("/artisan/run", {
      method: "POST",
      body: JSON.stringify({ project_path: projectPath, command }),
    });
  }

  async getArtisanCommands(): Promise<string[]> {
    return this.request<string[]>("/artisan/commands");
  }

  // Database Clone
  async cloneDatabase(source: string, target: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/db/clone", {
      method: "POST",
      body: JSON.stringify({ source, target }),
    });
  }

  // Database Relationships
  async getDbRelationships(database: string): Promise<
    {
      from_table: string;
      from_column: string;
      to_table: string;
      to_column: string;
    }[]
  > {
    return this.request(`/db/relationships?db=${encodeURIComponent(database)}`);
  }

  // Plugin Health & Logs
  async getPluginLogs(
    id: string,
    lines: number = 100,
  ): Promise<{ logs: string[] }> {
    return this.request<{ logs: string[] }>(
      `/plugins/logs?id=${id}&lines=${lines}`,
    );
  }

  async getPluginHealth(
    id: string,
  ): Promise<{ healthy: boolean; message: string }> {
    return this.request<{ healthy: boolean; message: string }>(
      `/plugins/health?id=${id}`,
    );
  }

  // ============ Supreme Healer ============
  async getHealerIssues(): Promise<HealerIssue[]> {
    const res = await this.request<HealerIssue[]>("/healer/issues");
    return res || [];
  }

  async resolveHealerIssue(id: string): Promise<ActionResponse> {
    return this.request<ActionResponse>("/healer/resolve", {
      method: "POST",
      body: JSON.stringify({ id }),
    });
  }
}

export const api = new DaemonApi();
