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

export interface ApiResponse<T = void> {
  success: boolean;
  message?: string;
  data?: T;
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
}

export interface HealthCheck {
  name: string;
  status: "pass" | "fail" | "warn";
  message: string;
  fixable?: boolean;
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
}

export interface ColumnInfo {
  name: string;
  type: string;
  nullable: boolean;
  key: string;
  default: string;
}

export interface TableData {
  columns: ColumnInfo[];
  rows: Record<string, any>[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface Snapshot {
  id: string;
  database: string;
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

// API Client
class DaemonApi {
  private async request<T>(
    endpoint: string,
    options?: RequestInit
  ): Promise<T> {
    const url = `${API_BASE}${endpoint}${
      endpoint.includes("?") ? "&" : "?"
    }t=${Date.now()}`;
    const response = await fetch(url, {
      headers: {
        "Content-Type": "application/json",
      },
      ...options,
    });

    if (!response.ok) {
      const error = await response
        .json()
        .catch(() => ({ error: "Unknown error" }));
      throw new Error(error.error || `Request failed: ${response.statusText}`);
    }

    return response.json();
  }

  // Database Management
  async getDatabases(): Promise<DatabaseInfo[]> {
    return this.request<DatabaseInfo[]>("/db/databases");
  }

  async getTables(database: string): Promise<TableInfo[]> {
    return this.request<TableInfo[]>(`/db/tables?db=${database}`);
  }

  async getTableData(
    database: string,
    table: string,
    page: number = 1
  ): Promise<TableData> {
    return this.request<TableData>(
      `/db/table?db=${database}&table=${table}&page=${page}`
    );
  }

  async getTableSchema(database: string, table: string): Promise<ColumnInfo[]> {
    return this.request<ColumnInfo[]>(
      `/db/schema?db=${database}&table=${table}`
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

  async executeQuery(database: string, query: string): Promise<QueryResult> {
    return this.request<QueryResult>("/db/query", {
      method: "POST",
      body: JSON.stringify({ database, query }),
    });
  }

  async importDatabase(
    file: File,
    database: string,
    restore: boolean = true
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
  async park(path: string): Promise<ApiResponse> {
    return this.request<ApiResponse>("/park", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
  }

  async forget(path: string): Promise<ApiResponse> {
    return this.request<ApiResponse>("/forget", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
  }

  async link(name: string, path: string): Promise<ApiResponse> {
    return this.request<ApiResponse>("/link", {
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
    const res = await this.request<ApiResponse>("/ignore", {
      method: "POST",
      body: JSON.stringify({ path }),
    });
    return res.success;
  }

  async unignore(path: string): Promise<boolean> {
    const res = await this.request<ApiResponse>("/unignore", {
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
    const res = await this.request<ApiResponse>("/plugins/install", {
      method: "POST",
      body: JSON.stringify({ id }),
    });
    return res.success;
  }

  async togglePlugin(id: string, enabled: boolean): Promise<boolean> {
    const res = await this.request<ApiResponse>("/plugins/toggle", {
      method: "POST",
      body: JSON.stringify({ id, enabled }),
    });
    return res.success;
  }

  // PHP Management
  async switchPHP(version: string): Promise<ApiResponse> {
    return this.request<ApiResponse>("/php", {
      method: "POST",
      body: JSON.stringify({ version }),
    });
  }

  // SSL/HTTPS
  async secure(): Promise<ApiResponse> {
    return this.request<ApiResponse>("/secure", {
      method: "POST",
    });
  }

  // Service Management (to be implemented in daemon)
  async getServiceStatus(): Promise<ServiceStatus[]> {
    try {
      const state = await this.getState();
      // Parse from state.services or mock for now
      return [
        { name: "Nginx", running: true, version: "1.24.0" },
        { name: "PHP-FPM", running: true, version: state.php_version || "8.2" },
        { name: "DNSMasq", running: true },
        { name: "MySQL", running: true, version: "8.0" },
      ];
    } catch {
      return [
        { name: "Nginx", running: false },
        { name: "PHP-FPM", running: false },
        { name: "DNSMasq", running: false },
        { name: "MySQL", running: false },
      ];
    }
  }

  async restart(): Promise<ApiResponse> {
    return this.request<ApiResponse>("/restart", {
      method: "POST",
    });
  }

  // Doctor/Diagnostics (to be implemented in daemon)
  async runDoctor(): Promise<HealthCheck[]> {
    // Mock for now - will be implemented in daemon
    return [
      {
        name: "Nginx Configuration",
        status: "pass",
        message: "Valid configuration",
      },
      {
        name: "PHP-FPM Socket",
        status: "pass",
        message: "Socket active and responding",
      },
      {
        name: "DNS Resolution",
        status: "pass",
        message: "Resolving .test domains correctly",
      },
      {
        name: "SSL Certificates",
        status: "pass",
        message: "mkcert CA installed",
      },
      { name: "Port 80", status: "pass", message: "No conflicts detected" },
      { name: "Port 443", status: "pass", message: "No conflicts detected" },
      {
        name: "MySQL Connection",
        status: "pass",
        message: "Database accessible",
      },
    ];
  }

  // Helper to transform state into projects list
  async getProjects(): Promise<Project[]> {
    const projects = await this.request<Project[]>("/sites");
    return projects || [];
  }
}

export const api = new DaemonApi();
