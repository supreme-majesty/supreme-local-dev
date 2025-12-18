import { useState } from "react";
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
  Loader2,
  Search,
  Code2,
  ChevronRight,
  ChevronDown,
} from "lucide-react";
import { formatBytes, formatDate, cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/Tabs";
import { SQLConsole } from "@/components/database/SQLConsole";

// Types
interface TableInfo {
  name: string;
  row_count: number;
  engine: string;
}

interface ColumnInfo {
  name: string;
  type: string;
  nullable: boolean;
  key: string;
  default: string;
}

interface TableData {
  columns: ColumnInfo[];
  rows: Record<string, any>[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

interface Snapshot {
  id: string;
  database: string;
  filename: string;
  size: number;
  created_at: string;
}

export default function Database() {
  const [selectedDB, setSelectedDB] = useState<string | null>(null);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState("data"); // data, structure, sql, snapshots
  const [searchTable, setSearchTable] = useState("");
  const queryClient = useQueryClient();

  // Queries
  const {
    data: databases,
    isLoading: loadingDBs,
    error: dbError,
  } = useQuery({
    queryKey: ["databases"],
    queryFn: async () => {
      const res = await fetch("/api/db/list");
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || "Failed to fetch databases");
      }
      return res.json() as Promise<string[]>;
    },
  });

  const { data: tables, isLoading: loadingTables } = useQuery({
    queryKey: ["tables", selectedDB],
    queryFn: async () => {
      if (!selectedDB) return [];
      const res = await fetch(`/api/db/tables?database=${selectedDB}`);
      if (!res.ok) throw new Error("Failed to fetch tables");
      return res.json() as Promise<TableInfo[]>;
    },
    enabled: !!selectedDB,
  });

  const { data: tableData, isLoading: loadingData } = useQuery({
    queryKey: ["tableData", selectedDB, selectedTable],
    queryFn: async () => {
      if (!selectedDB || !selectedTable) return null;
      const res = await fetch(
        `/api/db/data?database=${selectedDB}&table=${selectedTable}&page=1`
      );
      if (!res.ok) throw new Error("Failed to fetch data");
      return res.json() as Promise<TableData>;
    },
    enabled: !!selectedDB && !!selectedTable && activeTab === "data",
  });

  const { data: schema, isLoading: loadingSchema } = useQuery({
    queryKey: ["schema", selectedDB, selectedTable],
    queryFn: async () => {
      if (!selectedDB || !selectedTable) return [];
      const res = await fetch(
        `/api/db/schema?database=${selectedDB}&table=${selectedTable}`
      );
      if (!res.ok) throw new Error("Failed to fetch schema");
      return res.json() as Promise<ColumnInfo[]>;
    },
    enabled: !!selectedDB && !!selectedTable && activeTab === "structure",
  });

  const { data: snapshots, refetch: refetchSnapshots } = useQuery({
    queryKey: ["snapshots"],
    queryFn: async () => {
      const res = await fetch("/api/db/snapshots");
      if (!res.ok) throw new Error("Failed to fetch snapshots");
      return res.json() as Promise<Snapshot[]>;
    },
    enabled: activeTab === "snapshots",
  });

  // Mutations
  const createSnapshotMutation = useMutation({
    mutationFn: async (db: string) => {
      const res = await fetch("/api/db/snapshots", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ database: db }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || "Failed to create snapshot");
      }
    },
    onSuccess: () => {
      refetchSnapshots();
    },
  });

  const restoreSnapshotMutation = useMutation({
    mutationFn: async (snapshotId: string) => {
      const res = await fetch(`/api/db/snapshots/${snapshotId}/restore`, {
        method: "POST",
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || "Failed to restore snapshot");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["databases"] });
      queryClient.invalidateQueries({ queryKey: ["tables"] });
      queryClient.invalidateQueries({ queryKey: ["tableData"] });
      queryClient.invalidateQueries({ queryKey: ["schema"] });
    },
  });

  const deleteSnapshotMutation = useMutation({
    mutationFn: async (snapshotId: string) => {
      const res = await fetch(`/api/db/snapshots/${snapshotId}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || "Failed to delete snapshot");
      }
    },
    onSuccess: () => {
      refetchSnapshots();
    },
  });

  // Filtered Tables
  const filteredTables =
    tables?.filter((t) =>
      t.name.toLowerCase().includes(searchTable.toLowerCase())
    ) || [];

  if (dbError) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <Card className="max-w-md w-full border-red-500/20 bg-red-500/5">
          <CardHeader className="text-center">
            <div className="mx-auto w-12 h-12 bg-red-500/10 rounded-full flex items-center justify-center mb-4 text-red-500">
              <AlertCircle />
            </div>
            <CardTitle className="text-red-500">Connection Failed</CardTitle>
          </CardHeader>
          <CardContent className="text-center text-sm text-[var(--muted-foreground)] space-y-4">
            <p>{(dbError as Error).message}</p>
            <p>
              Ensure the daemon is running and has MySQL access permissions.
            </p>
            <Button variant="secondary" onClick={() => window.location.reload()}>
              Retry Connection
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex h-screen bg-[var(--background)] overflow-hidden">
      {/* Sidebar - Database & Table Tree */}
      <div className="w-64 flex-shrink-0 border-r border-[var(--border)] bg-[var(--card)]/50 backdrop-blur-sm flex flex-col">
        <div className="p-4 border-b border-[var(--border)]">
          <h2 className="font-semibold flex items-center gap-2 mb-4 text-[var(--foreground)]">
            <DatabaseIcon className="w-4 h-4 text-blue-500" />
            Connections
          </h2>
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-[var(--muted-foreground)]" />
            <Input
              placeholder="Search tables..."
              className="pl-9 h-9 text-xs"
              value={searchTable}
              onChange={(e) => setSearchTable(e.target.value)}
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-2 space-y-1">
          {loadingDBs ? (
            <div className="flex justify-center p-4">
              <Loader2 className="animate-spin text-[var(--muted-foreground)]" />
            </div>
          ) : (
            databases?.map((db) => (
              <div key={db} className="space-y-1">
                <button
                  onClick={() => {
                    setSelectedDB(db === selectedDB ? null : db);
                    if (db !== selectedDB) {
                      setSelectedTable(null);
                      setActiveTab("data");
                    }
                  }}
                  className={cn(
                    "w-full flex items-center gap-2 px-3 py-2 rounded-md text-sm transition-colors duration-200 group text-left",
                    selectedDB === db
                      ? "bg-blue-500/10 text-blue-500 font-medium"
                      : "text-[var(--muted-foreground)] hover:bg-[var(--card-hover)] hover:text-[var(--foreground)]"
                  )}
                >
                  {selectedDB === db ? (
                    <ChevronDown size={14} />
                  ) : (
                    <ChevronRight size={14} />
                  )}
                  <DatabaseIcon
                    size={14}
                    className={cn(
                      "group-hover:text-blue-500 transition-colors",
                      selectedDB === db
                        ? "text-blue-500"
                        : "text-[var(--muted-foreground)]"
                    )}
                  />
                  <span className="truncate">{db}</span>
                </button>

                {selectedDB === db && (
                  <div className="ml-4 pl-2 border-l border-[var(--border)]">
                    {loadingTables ? (
                      <div className="py-2 pl-4 text-xs text-[var(--muted-foreground)]">
                        Loading tables...
                      </div>
                    ) : (
                      filteredTables.map((table) => (
                        <button
                          key={table.name}
                          onClick={() => {
                            setSelectedTable(table.name);
                            setActiveTab("data");
                          }}
                          className={cn(
                            "w-full flex items-center gap-2 px-3 py-1.5 rounded-md text-sm transition-all duration-200 text-left",
                            selectedTable === table.name
                              ? "bg-[var(--card-hover)] text-[var(--foreground)] font-medium"
                              : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                          )}
                        >
                          <Table2
                            size={14}
                            className={
                              selectedTable === table.name
                                ? "text-emerald-500"
                                : "opacity-50"
                            }
                          />
                          <span className="truncate">{table.name}</span>
                          <span className="ml-auto text-[10px] opacity-40 tabular-nums">
                            {table.row_count}
                          </span>
                        </button>
                      ))
                    )}
                    {filteredTables.length === 0 && !loadingTables && (
                      <div className="py-2 pl-4 text-xs text-[var(--muted-foreground)] italic">
                        No tables found
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col min-w-0 bg-[var(--background)]">
        {selectedDB ? (
          <>
            {/* Header */}
            <div className="h-14 border-b border-[var(--border)] px-6 flex items-center justify-between bg-[var(--card)]/30 backdrop-blur-md">
              <div className="flex items-center gap-2 overflow-hidden">
                <DatabaseIcon className="w-4 h-4 text-blue-500 flex-shrink-0" />
                <span className="font-medium text-[var(--muted-foreground)] whitespace-nowrap">
                  {selectedDB}
                </span>
                {selectedTable && (
                  <>
                    <ChevronRight className="w-4 h-4 text-[var(--muted-foreground)] flex-shrink-0" />
                    <Table2 className="w-4 h-4 text-emerald-500 flex-shrink-0" />
                    <span className="font-semibold text-[var(--foreground)] truncate">
                      {selectedTable}
                    </span>
                  </>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="secondary" className="h-8 shadow-sm">
                  <RefreshCw className="w-3.5 h-3.5 mr-2" />
                  Refresh
                </Button>
                <Button
                  size="sm"
                  className="h-8 bg-blue-600 hover:bg-blue-700 text-white shadow-md shadow-blue-500/20"
                  onClick={() => createSnapshotMutation.mutate(selectedDB)}
                  loading={createSnapshotMutation.isPending}
                >
                  <Camera className="w-3.5 h-3.5 mr-2" />
                  Snapshot
                </Button>
              </div>
            </div>

            {/* Content Tabs */}
            <Tabs
              value={activeTab}
              onValueChange={setActiveTab}
              className="flex-1 flex flex-col min-h-0"
            >
              <div className="px-6 border-b border-[var(--border)] bg-[var(--muted)]/20">
                <TabsList className="bg-transparent -mb-px p-0 h-10 gap-6">
                  <TabsTrigger
                    value="data"
                    disabled={!selectedTable}
                    className="relative h-10 rounded-none border-b-2 border-transparent bg-transparent px-0 pb-2 pt-2 font-medium text-[var(--muted-foreground)] shadow-none transition-none data-[state=active]:border-blue-500 data-[state=active]:text-blue-500 data-[state=active]:shadow-none disabled:opacity-30"
                  >
                    Table Data
                  </TabsTrigger>
                  <TabsTrigger
                    value="structure"
                    disabled={!selectedTable}
                    className="relative h-10 rounded-none border-b-2 border-transparent bg-transparent px-0 pb-2 pt-2 font-medium text-[var(--muted-foreground)] shadow-none transition-none data-[state=active]:border-blue-500 data-[state=active]:text-blue-500 data-[state=active]:shadow-none disabled:opacity-30"
                  >
                    Structure
                  </TabsTrigger>
                  <TabsTrigger
                    value="sql"
                    className="relative h-10 rounded-none border-b-2 border-transparent bg-transparent px-0 pb-2 pt-2 font-medium text-[var(--muted-foreground)] shadow-none transition-none data-[state=active]:border-blue-500 data-[state=active]:text-blue-500 data-[state=active]:shadow-none"
                  >
                    <Code2 className="w-4 h-4 mr-2" />
                    SQL Console
                  </TabsTrigger>
                  <TabsTrigger
                    value="snapshots"
                    className="relative h-10 rounded-none border-b-2 border-transparent bg-transparent px-0 pb-2 pt-2 font-medium text-[var(--muted-foreground)] shadow-none transition-none data-[state=active]:border-blue-500 data-[state=active]:text-blue-500 data-[state=active]:shadow-none"
                  >
                    Snapshots
                  </TabsTrigger>
                </TabsList>
              </div>

              {/* Tab Views */}
              <div className="flex-1 overflow-hidden p-6 ">
                <TabsContent
                  value="data"
                  className="h-full m-0 data-[state=active]:flex flex-col border border-[var(--border)] rounded-lg bg-[var(--card)] shadow-sm"
                >
                  {selectedTable ? (
                    loadingData ? (
                      <div className="flex h-full items-center justify-center">
                        <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
                      </div>
                    ) : tableData ? (
                      <div className="flex-1 overflow-auto rounded-lg">
                        <table className="w-full text-sm">
                          <thead className="sticky top-0 bg-[var(--muted)]/80 backdrop-blur-sm z-10 shadow-sm">
                            <tr>
                              {tableData.columns.map((col) => (
                                <th
                                  key={col.name}
                                  className="px-4 py-3 text-left font-semibold text-[var(--muted-foreground)] border-b border-[var(--border)] whitespace-nowrap"
                                >
                                  <div className="flex items-center gap-2">
                                    {col.name}
                                    {col.key === "PRI" && (
                                      <Badge
                                        variant="secondary"
                                        className="h-4 px-1 text-[10px] bg-yellow-500/10 text-yellow-600 border-yellow-500/20"
                                      >
                                        PK
                                      </Badge>
                                    )}
                                  </div>
                                </th>
                              ))}
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-[var(--border)]">
                            {tableData.rows.map((row, i) => (
                              <tr
                                key={i}
                                className="hover:bg-[var(--muted)]/30 transition-colors"
                              >
                                {tableData.columns.map((col) => (
                                  <td
                                    key={col.name}
                                    className="px-4 py-2.5 whitespace-nowrap font-mono text-xs text-[var(--foreground)]"
                                  >
                                    {row[col.name] === null ? (
                                      <span className="text-[var(--muted-foreground)] italic opacity-50">
                                        NULL
                                      </span>
                                    ) : (
                                      String(row[col.name])
                                    )}
                                  </td>
                                ))}
                              </tr>
                            ))}
                            {tableData.rows.length === 0 && (
                              <tr>
                                <td
                                  colSpan={tableData.columns.length}
                                  className="px-4 py-12 text-center text-[var(--muted-foreground)]"
                                >
                                  Table is empty
                                </td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    ) : null
                  ) : (
                    <div className="flex h-full flex-col items-center justify-center text-[var(--muted-foreground)]">
                      <Table2 className="w-12 h-12 mb-4 opacity-20" />
                      <p>Select a table to view data</p>
                    </div>
                  )}
                </TabsContent>

                <TabsContent
                  value="structure"
                  className="h-full m-0 data-[state=active]:flex flex-col border border-[var(--border)] rounded-lg bg-[var(--card)] shadow-sm p-4"
                >
                  {selectedTable ? (
                    loadingSchema ? (
                      <div className="flex h-full items-center justify-center">
                        <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
                      </div>
                    ) : schema && schema.length > 0 ? (
                      <div className="space-y-2 overflow-auto">
                        {schema.map((col) => (
                          <div
                            key={col.name}
                            className="flex items-center gap-4 p-3 bg-[var(--background)] rounded-lg border border-[var(--border)]"
                          >
                            <div className="flex-1">
                              <div className="flex items-center gap-2">
                                <span className="font-mono font-medium">
                                  {col.name}
                                </span>
                                {col.key === "PRI" && (
                                  <Badge
                                    variant="secondary"
                                    className="h-4 px-1 text-[10px] bg-yellow-500/10 text-yellow-600 border-yellow-500/20"
                                  >
                                    PRIMARY
                                  </Badge>
                                )}
                                {col.key === "UNI" && (
                                  <Badge
                                    variant="secondary"
                                    className="h-4 px-1 text-[10px] bg-blue-500/10 text-blue-600 border-blue-500/20"
                                  >
                                    UNIQUE
                                  </Badge>
                                )}
                              </div>
                              <div className="text-xs text-[var(--muted-foreground)] mt-1 font-mono">
                                {col.type}
                                {col.nullable && " • nullable"}
                                {col.default && ` • default: ${col.default}`}
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="flex h-full flex-col items-center justify-center text-[var(--muted-foreground)]">
                        <Columns className="w-12 h-12 mb-4 opacity-20" />
                        <p>No schema information available for this table.</p>
                      </div>
                    )
                  ) : (
                    <div className="flex h-full flex-col items-center justify-center text-[var(--muted-foreground)]">
                      <Columns className="w-12 h-12 mb-4 opacity-20" />
                      <p>Select a table to view its structure</p>
                    </div>
                  )}
                </TabsContent>

                <TabsContent value="sql" className="h-full m-0">
                  <SQLConsole database={selectedDB} />
                </TabsContent>

                <TabsContent value="snapshots" className="h-full m-0 space-y-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                    {!snapshots?.filter((s) => s.database === selectedDB)
                      .length ? (
                      <div className="col-span-full py-12 text-center border border-dashed border-[var(--border)] rounded-lg">
                        <Camera className="w-12 h-12 mx-auto mb-4 text-[var(--muted-foreground)] opacity-20" />
                        <h3 className="font-semibold mb-1">No Snapshots</h3>
                        <p className="text-sm text-[var(--muted-foreground)] mb-4">
                          Create a backup of this database
                        </p>
                        <Button
                          size="sm"
                          onClick={() =>
                            createSnapshotMutation.mutate(selectedDB)
                          }
                          loading={createSnapshotMutation.isPending}
                        >
                          Create Snapshot
                        </Button>
                      </div>
                    ) : (
                      snapshots
                        ?.filter((s) => s.database === selectedDB)
                        .map((snap) => (
                          <Card
                            key={snap.id}
                            className="hover:shadow-lg transition-all duration-300 border-[var(--border)] bg-[var(--card)] group cursor-pointer"
                          >
                            <CardHeader className="p-4 pb-2 flex flex-row items-center justify-between space-y-0">
                              <div className="flex items-center gap-2">
                                <div className="p-2 bg-blue-500/10 rounded-lg text-blue-500">
                                  <HardDrive size={16} />
                                </div>
                                <div className="font-semibold text-sm truncate max-w-[150px]">
                                  {snap.filename}
                                </div>
                              </div>
                              <Badge
                                variant="secondary"
                                className="font-mono text-xs"
                              >
                                {formatBytes(snap.size)}
                              </Badge>
                            </CardHeader>
                            <CardContent className="p-4 pt-2">
                              <div className="flex items-center text-xs text-[var(--muted-foreground)] mb-4">
                                <Clock size={12} className="mr-1" />
                                {formatDate(snap.created_at)}
                              </div>
                                                            <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                                                <Button
                                                    size="sm"
                                                    variant="secondary"
                                                    className="flex-1 h-8 text-xs"
                                                    onClick={() => restoreSnapshotMutation.mutate(snap.id)}
                                                    loading={restoreSnapshotMutation.isPending}
                                                >
                                                    <RotateCcw size={12} className="mr-1" />
                                                    Restore
                                                </Button>
                                                <Button
                                                    size="sm"
                                                    variant="danger"
                                                    className="h-8 w-8 p-0"
                                                    onClick={() => deleteSnapshotMutation.mutate(snap.id)}
                                                    loading={deleteSnapshotMutation.isPending}
                                                >
                                                    <Trash2 size={12} />
                                                </Button>
                                            </div>                     </CardContent>
                          </Card>
                        ))
                    )}
                  </div>
                </TabsContent>
              </div>
            </Tabs>
          </>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center text-[var(--muted-foreground)] bg-dot-pattern">
            <div className="w-24 h-24 rounded-full bg-blue-500/5 flex items-center justify-center mb-6 animate-pulse">
              <DatabaseIcon className="w-10 h-10 text-blue-500" />
            </div>
            <h2 className="text-xl font-semibold text-[var(--foreground)] mb-2">
              Select a Database
            </h2>
            <p className="max-w-xs text-center text-sm">
              Choose a database from the sidebar to view tables, run queries,
              and manage snapshots.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
