import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Database as DatabaseIcon,
  Table2,
  RefreshCw,
  Camera,
  RotateCcw,
  Trash2,
  Columns,
  HardDrive,
  Clock,
  AlertCircle,
  CheckCircle,
  Loader2,
} from "lucide-react";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/stores/useAppStore";

// Types
interface DatabaseInfo {
  name: string;
  tables: number;
}

interface TableInfo {
  name: string;
  rows: number;
  size: string;
  engine: string;
}

interface ColumnInfo {
  name: string;
  type: string;
  nullable: boolean;
  key: string;
  default: string | null;
  extra: string;
}

interface TableData {
  columns: string[];
  rows: (string | number | null)[][];
  total: number;
  limit: number;
  offset: number;
}

interface Snapshot {
  id: string;
  name: string;
  database: string;
  size: number;
  created_at: string;
  path: string;
}

interface DBStatus {
  connected: boolean;
  host: string;
  port: string;
  user: string;
  version?: string;
  error?: string;
}

// API functions
const API_BASE = "/api";

async function fetchDBStatus(): Promise<DBStatus> {
  const res = await fetch(`${API_BASE}/db/status`);
  return res.json();
}

async function fetchDatabases(): Promise<DatabaseInfo[]> {
  const res = await fetch(`${API_BASE}/db/databases`);
  return res.json();
}

async function fetchTables(db: string): Promise<TableInfo[]> {
  const res = await fetch(`${API_BASE}/db/tables?db=${db}`);
  return res.json();
}

async function fetchTableData(
  db: string,
  table: string,
  limit = 50,
  offset = 0
): Promise<TableData> {
  const res = await fetch(
    `${API_BASE}/db/table?db=${db}&table=${table}&limit=${limit}&offset=${offset}`
  );
  return res.json();
}

async function fetchSchema(db: string, table: string): Promise<ColumnInfo[]> {
  const res = await fetch(`${API_BASE}/db/schema?db=${db}&table=${table}`);
  return res.json();
}

async function fetchSnapshots(db?: string): Promise<Snapshot[]> {
  const url = db
    ? `${API_BASE}/db/snapshots?db=${db}`
    : `${API_BASE}/db/snapshots`;
  const res = await fetch(url);
  return res.json();
}

async function createSnapshot(
  database: string,
  name: string
): Promise<Snapshot> {
  const res = await fetch(`${API_BASE}/db/snapshots`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, name }),
  });
  return res.json();
}

async function restoreSnapshot(database: string, path: string): Promise<void> {
  await fetch(`${API_BASE}/db/snapshots/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, path }),
  });
}

async function deleteSnapshot(id: string): Promise<void> {
  await fetch(`${API_BASE}/db/snapshots`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
  });
}

// Status Badge Component
function StatusBadge({
  connected,
  version,
}: {
  connected: boolean;
  version?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 py-1.5 rounded-full text-sm font-medium",
        connected
          ? "bg-green-500/10 text-green-400 border border-green-500/20"
          : "bg-red-500/10 text-red-400 border border-red-500/20"
      )}
    >
      {connected ? <CheckCircle size={14} /> : <AlertCircle size={14} />}
      {connected ? `MySQL ${version || ""}` : "Disconnected"}
    </div>
  );
}

// Database Selector
function DatabaseSelector({
  databases,
  selected,
  onSelect,
}: {
  databases: DatabaseInfo[];
  selected: string;
  onSelect: (db: string) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {databases.map((db) => (
        <button
          key={db.name}
          onClick={() => onSelect(db.name)}
          className={cn(
            "px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200",
            selected === db.name
              ? "bg-[var(--primary)] text-[var(--primary-foreground)] shadow-lg"
              : "bg-[var(--card)] border border-[var(--border)] hover:border-[var(--primary)]/50"
          )}
        >
          <div className="flex items-center gap-2">
            <DatabaseIcon size={14} />
            <span>{db.name}</span>
            <span className="text-xs opacity-60">({db.tables})</span>
          </div>
        </button>
      ))}
    </div>
  );
}

// Table List
function TableList({
  tables,
  selected,
  onSelect,
}: {
  tables: TableInfo[];
  selected: string;
  onSelect: (table: string) => void;
}) {
  return (
    <div className="space-y-1">
      {tables.map((table) => (
        <button
          key={table.name}
          onClick={() => onSelect(table.name)}
          className={cn(
            "w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-all",
            selected === table.name
              ? "bg-[var(--primary)]/10 text-[var(--primary)] border-l-2 border-[var(--primary)]"
              : "hover:bg-[var(--card-hover)] text-[var(--muted-foreground)]"
          )}
        >
          <div className="flex items-center gap-2">
            <Table2 size={14} />
            <span className="font-mono">{table.name}</span>
          </div>
          <span className="text-xs opacity-50">{table.rows}</span>
        </button>
      ))}
    </div>
  );
}

// Data Table
function DataTable({ data }: { data: TableData }) {
  if (!data.rows || data.rows.length === 0) {
    return (
      <div className="text-center py-12 text-[var(--muted-foreground)]">
        No data in this table
      </div>
    );
  }

  return (
    <div className="overflow-auto max-h-[500px] rounded-lg border border-[var(--border)]">
      <table className="w-full text-sm">
        <thead className="bg-[var(--card)] sticky top-0">
          <tr>
            {data.columns.map((col) => (
              <th
                key={col}
                className="px-4 py-3 text-left font-medium text-[var(--muted-foreground)] border-b border-[var(--border)] whitespace-nowrap"
              >
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.rows.map((row, i) => (
            <tr
              key={i}
              className="hover:bg-[var(--card-hover)] transition-colors"
            >
              {row.map((cell, j) => (
                <td
                  key={j}
                  className="px-4 py-2 border-b border-[var(--border)] whitespace-nowrap max-w-[300px] truncate"
                  title={String(cell ?? "")}
                >
                  {cell === null ? (
                    <span className="text-[var(--muted-foreground)] italic">
                      NULL
                    </span>
                  ) : (
                    String(cell)
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Schema View
function SchemaView({ schema }: { schema: ColumnInfo[] }) {
  return (
    <div className="space-y-2">
      {schema.map((col) => (
        <div
          key={col.name}
          className="flex items-center gap-4 p-3 bg-[var(--card)] rounded-lg border border-[var(--border)]"
        >
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <span className="font-mono font-medium">{col.name}</span>
              {col.key === "PRI" && (
                <span className="px-1.5 py-0.5 text-[10px] bg-yellow-500/10 text-yellow-400 rounded">
                  PRIMARY
                </span>
              )}
              {col.key === "UNI" && (
                <span className="px-1.5 py-0.5 text-[10px] bg-blue-500/10 text-blue-400 rounded">
                  UNIQUE
                </span>
              )}
            </div>
            <div className="text-xs text-[var(--muted-foreground)] mt-1 font-mono">
              {col.type}
              {col.nullable && " • nullable"}
              {col.extra && ` • ${col.extra}`}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// Snapshot Panel
function SnapshotPanel({
  database,
  snapshots,
  onRefresh,
}: {
  database: string;
  snapshots: Snapshot[];
  onRefresh: () => void;
}) {
  const [snapshotName, setSnapshotName] = useState("");
  const addToast = useAppStore((s) => s.addToast);
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: () => createSnapshot(database, snapshotName),
    onSuccess: () => {
      addToast({ type: "success", title: "Snapshot created" });
      setSnapshotName("");
      queryClient.invalidateQueries({ queryKey: ["db-snapshots"] });
      onRefresh();
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to create snapshot",
        description: err.message,
      });
    },
  });

  const restoreMutation = useMutation({
    mutationFn: (path: string) => restoreSnapshot(database, path),
    onSuccess: () => {
      addToast({ type: "success", title: "Database restored" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to restore",
        description: err.message,
      });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSnapshot(id),
    onSuccess: () => {
      addToast({ type: "success", title: "Snapshot deleted" });
      queryClient.invalidateQueries({ queryKey: ["db-snapshots"] });
      onRefresh();
    },
  });

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString();
  };

  return (
    <div className="space-y-4">
      {/* Create Snapshot */}
      <div className="flex gap-2">
        <input
          type="text"
          value={snapshotName}
          onChange={(e) => setSnapshotName(e.target.value)}
          placeholder="Snapshot name (optional)"
          className="flex-1 px-4 py-2 bg-[var(--background)] border border-[var(--border)] rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-[var(--primary)]/50"
        />
        <Button
          onClick={() => createMutation.mutate()}
          loading={createMutation.isPending}
          disabled={!database}
        >
          <Camera size={14} />
          Create Snapshot
        </Button>
      </div>

      {/* Snapshot List */}
      <div className="space-y-2">
        {snapshots.length === 0 && (
          <div className="text-center py-8 text-[var(--muted-foreground)]">
            No snapshots for this database
          </div>
        )}
        {snapshots.map((snap) => (
          <div
            key={snap.id}
            className="flex items-center justify-between p-4 bg-[var(--card)] rounded-lg border border-[var(--border)]"
          >
            <div>
              <div className="font-medium">{snap.name}</div>
              <div className="flex items-center gap-4 text-xs text-[var(--muted-foreground)] mt-1">
                <span className="flex items-center gap-1">
                  <HardDrive size={12} />
                  {formatSize(snap.size)}
                </span>
                <span className="flex items-center gap-1">
                  <Clock size={12} />
                  {formatDate(snap.created_at)}
                </span>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                onClick={() => restoreMutation.mutate(snap.path)}
                loading={restoreMutation.isPending}
              >
                <RotateCcw size={14} />
                Restore
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => deleteMutation.mutate(snap.id)}
                loading={deleteMutation.isPending}
              >
                <Trash2 size={14} />
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// Main Component
export default function Database() {
  const [selectedDb, setSelectedDb] = useState("");
  const [selectedTable, setSelectedTable] = useState("");
  const [activeTab, setActiveTab] = useState<"data" | "schema" | "snapshots">(
    "data"
  );
  const [page, setPage] = useState(0);

  // Queries
  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ["db-status"],
    queryFn: fetchDBStatus,
    refetchInterval: 30000,
  });

  const { data: databases = [], refetch: refetchDatabases } = useQuery({
    queryKey: ["db-databases"],
    queryFn: fetchDatabases,
    enabled: status?.connected,
  });

  const { data: tables = [] } = useQuery({
    queryKey: ["db-tables", selectedDb],
    queryFn: () => fetchTables(selectedDb),
    enabled: !!selectedDb,
  });

  const { data: tableData, isLoading: dataLoading } = useQuery({
    queryKey: ["db-table-data", selectedDb, selectedTable, page],
    queryFn: () => fetchTableData(selectedDb, selectedTable, 50, page * 50),
    enabled: !!selectedDb && !!selectedTable && activeTab === "data",
  });

  const { data: schema = [] } = useQuery({
    queryKey: ["db-schema", selectedDb, selectedTable],
    queryFn: () => fetchSchema(selectedDb, selectedTable),
    enabled: !!selectedDb && !!selectedTable && activeTab === "schema",
  });

  const { data: snapshots = [], refetch: refetchSnapshots } = useQuery({
    queryKey: ["db-snapshots", selectedDb],
    queryFn: () => fetchSnapshots(selectedDb),
    enabled: !!selectedDb && activeTab === "snapshots",
  });

  // Reset table selection when database changes
  useEffect(() => {
    setSelectedTable("");
    setPage(0);
  }, [selectedDb]);

  if (statusLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-[var(--primary)]" />
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Database</h1>
          <p className="text-[var(--muted-foreground)]">
            Browse and manage your local databases
          </p>
        </div>
        <div className="flex items-center gap-3">
          <StatusBadge
            connected={status?.connected ?? false}
            version={status?.version}
          />
          <Button variant="secondary" onClick={() => refetchDatabases()}>
            <RefreshCw size={16} />
            Refresh
          </Button>
        </div>
      </div>

      {/* Connection Error */}
      {status && !status.connected && (
        <Card>
          <div className="p-6 text-center">
            <AlertCircle
              size={48}
              className="mx-auto mb-4 text-red-400 opacity-50"
            />
            <h3 className="font-semibold text-lg mb-2">
              Cannot Connect to MySQL
            </h3>
            <p className="text-[var(--muted-foreground)] mb-4">
              {status.error || "Make sure MySQL is running and accessible."}
            </p>
            <p className="text-sm text-[var(--muted-foreground)]">
              Connection: {status.user}@{status.host}:{status.port}
            </p>
          </div>
        </Card>
      )}

      {/* Main Content */}
      {status?.connected && (
        <>
          {/* Database Selector */}
          <DatabaseSelector
            databases={databases}
            selected={selectedDb}
            onSelect={setSelectedDb}
          />

          {selectedDb && (
            <div className="grid grid-cols-12 gap-6">
              {/* Table List Sidebar */}
              <div className="col-span-3">
                <Card>
                  <div className="p-4">
                    <h3 className="text-sm font-medium text-[var(--muted-foreground)] mb-3">
                      Tables ({tables.length})
                    </h3>
                    <TableList
                      tables={tables}
                      selected={selectedTable}
                      onSelect={setSelectedTable}
                    />
                  </div>
                </Card>
              </div>

              {/* Main Panel */}
              <div className="col-span-9">
                {/* Tabs */}
                <div className="flex items-center gap-1 mb-4 p-1 bg-[var(--card)] rounded-lg w-fit">
                  {(["data", "schema", "snapshots"] as const).map((tab) => (
                    <button
                      key={tab}
                      onClick={() => setActiveTab(tab)}
                      className={cn(
                        "px-4 py-2 rounded-md text-sm font-medium transition-all",
                        activeTab === tab
                          ? "bg-[var(--primary)] text-[var(--primary-foreground)]"
                          : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                      )}
                    >
                      {tab === "data" && (
                        <Table2 size={14} className="inline mr-2" />
                      )}
                      {tab === "schema" && (
                        <Columns size={14} className="inline mr-2" />
                      )}
                      {tab === "snapshots" && (
                        <Camera size={14} className="inline mr-2" />
                      )}
                      {tab.charAt(0).toUpperCase() + tab.slice(1)}
                    </button>
                  ))}
                </div>

                {/* Tab Content */}
                <Card>
                  <div className="p-4">
                    {activeTab === "data" && (
                      <>
                        {!selectedTable ? (
                          <div className="text-center py-12 text-[var(--muted-foreground)]">
                            Select a table to view data
                          </div>
                        ) : dataLoading ? (
                          <div className="flex items-center justify-center py-12">
                            <Loader2 className="w-6 h-6 animate-spin text-[var(--primary)]" />
                          </div>
                        ) : tableData ? (
                          <>
                            <DataTable data={tableData} />
                            {/* Pagination */}
                            {tableData.total > tableData.limit && (
                              <div className="flex items-center justify-between mt-4 pt-4 border-t border-[var(--border)]">
                                <span className="text-sm text-[var(--muted-foreground)]">
                                  Showing {tableData.offset + 1}-
                                  {Math.min(
                                    tableData.offset + tableData.limit,
                                    tableData.total
                                  )}{" "}
                                  of {tableData.total}
                                </span>
                                <div className="flex gap-2">
                                  <Button
                                    size="sm"
                                    variant="secondary"
                                    disabled={page === 0}
                                    onClick={() => setPage((p) => p - 1)}
                                  >
                                    Previous
                                  </Button>
                                  <Button
                                    size="sm"
                                    variant="secondary"
                                    disabled={
                                      tableData.offset + tableData.limit >=
                                      tableData.total
                                    }
                                    onClick={() => setPage((p) => p + 1)}
                                  >
                                    Next
                                  </Button>
                                </div>
                              </div>
                            )}
                          </>
                        ) : null}
                      </>
                    )}

                    {activeTab === "schema" && (
                      <>
                        {!selectedTable ? (
                          <div className="text-center py-12 text-[var(--muted-foreground)]">
                            Select a table to view schema
                          </div>
                        ) : (
                          <SchemaView schema={schema} />
                        )}
                      </>
                    )}

                    {activeTab === "snapshots" && (
                      <SnapshotPanel
                        database={selectedDb}
                        snapshots={snapshots}
                        onRefresh={() => refetchSnapshots()}
                      />
                    )}
                  </div>
                </Card>
              </div>
            </div>
          )}

          {!selectedDb && databases.length > 0 && (
            <Card>
              <div className="text-center py-12 text-[var(--muted-foreground)]">
                <DatabaseIcon size={48} className="mx-auto mb-4 opacity-30" />
                <p>Select a database to get started</p>
              </div>
            </Card>
          )}

          {databases.length === 0 && (
            <Card>
              <div className="text-center py-12 text-[var(--muted-foreground)]">
                <DatabaseIcon size={48} className="mx-auto mb-4 opacity-30" />
                <p>No databases found</p>
              </div>
            </Card>
          )}
        </>
      )}
    </div>
  );
}
