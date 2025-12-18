import { useState, useEffect, useRef } from "react";
import {
  Database as DatabaseIcon,
  Table as TableIcon,
  Server,
  RefreshCw,
  Plus,
  Trash2,
  HardDrive,
  Code2,
  Settings,
  ArrowRight,
  FileDown,
  FileUp,
  Camera,
  RotateCcw,
  Columns,
} from "lucide-react";
import { formatBytes, formatDate, cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { SQLConsole } from "@/components/database/SQLConsole";
// @ts-ignore
import { DatabaseTree } from "@/components/database/DatabaseTree";
import { DataForm } from "@/components/database/DataForm";
import {
  useTableData,
  useTableColumns,
  useSnapshots,
  useCreateSnapshotMutation,
  useRestoreSnapshotMutation,
  useDeleteSnapshotMutation,
  useExecuteQueryMutation,
  useImportDatabaseMutation,
} from "@/hooks/use-database";

export default function Database() {
  const [selectedDB, setSelectedDB] = useState<string | null>(null);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState("browse");
  const [page, setPage] = useState(1);
  const [editingRow, setEditingRow] = useState<Record<string, any> | null>(
    null
  );

  const fileInputRef = useRef<HTMLInputElement>(null);

  // Queries
  const { data: tableData, isLoading: loadingData } = useTableData(
    selectedDB,
    selectedTable,
    page
  );
  const { data: tableSchema } = useTableColumns(selectedDB, selectedTable);
  const { data: snapshots } = useSnapshots();

  // Helpers
  const pkCol = tableSchema?.find((c) => c.key === "PRI")?.name;

  const escapeValue = (val: any) => {
    if (val === null) return "NULL";
    if (typeof val === "number") return val;
    return `'${String(val).replace(/'/g, "\\'")}'`;
  };

  // Mutations
  const createSnapshotMutation = useCreateSnapshotMutation();
  const restoreSnapshotMutation = useRestoreSnapshotMutation();
  const deleteSnapshotMutation = useDeleteSnapshotMutation();
  const executeQueryMutation = useExecuteQueryMutation();
  const importDatabaseMutation = useImportDatabaseMutation();

  const handleImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      importDatabaseMutation.mutate(file);
      // Reset input?
      e.target.value = "";
    }
  };

  const handleDeleteRow = (row: Record<string, any>) => {
    if (!selectedDB || !selectedTable || !pkCol) return;

    if (!confirm("Are you sure you want to delete this row?")) return;

    const query = `DELETE FROM \`${selectedTable}\` WHERE \`${pkCol}\` = ${escapeValue(
      row[pkCol]
    )} LIMIT 1`;
    executeQueryMutation.mutate({ database: selectedDB, query });
  };

  const handleSaveRow = (data: Record<string, any>) => {
    if (!selectedDB || !selectedTable) return;

    let query = "";
    if (activeTab === "edit" && editingRow && pkCol) {
      // UPDATE
      const updates = Object.entries(data)
        .filter(([k, v]) => v !== editingRow[k]) // Only changed fields
        .map(([k, v]) => `\`${k}\` = ${escapeValue(v)}`)
        .join(", ");

      if (!updates) {
        setActiveTab("browse");
        return;
      }

      query = `UPDATE \`${selectedTable}\` SET ${updates} WHERE \`${pkCol}\` = ${escapeValue(
        editingRow[pkCol]
      )} LIMIT 1`;
    } else {
      // INSERT
      const cols = Object.keys(data)
        .map((k) => `\`${k}\``)
        .join(", ");
      const vals = Object.values(data)
        .map((v) => escapeValue(v))
        .join(", ");
      query = `INSERT INTO \`${selectedTable}\` (${cols}) VALUES (${vals})`;
    }

    executeQueryMutation.mutate(
      { database: selectedDB, query },
      {
        onSuccess: () => {
          setActiveTab("browse");
          setEditingRow(null);
        },
      }
    );
  };

  const startEdit = (row: Record<string, any>) => {
    setEditingRow(row);
    setActiveTab("edit");
  };

  // Reset page when table changes
  useEffect(() => {
    setPage(1);
    if (!selectedTable && selectedDB) {
      setActiveTab("structure"); // DB view default
    } else if (selectedTable) {
      setActiveTab("browse"); // Table view default
    }
  }, [selectedDB, selectedTable]);

  const handleSelectDb = (db: string) => {
    setSelectedDB(db);
    setSelectedTable(null);
  };

  const handleSelectTable = (db: string, table: string) => {
    setSelectedDB(db);
    setSelectedTable(table);
  };

  const currentSnapshotList =
    snapshots?.filter((s) => s.database === selectedDB) || [];

  return (
    <div className="flex h-[calc(100vh-6rem)] -m-6 bg-[var(--background)] animate-fade-in relative">
      {/* Sidebar - Database Tree */}
      <div className="w-64 border-r border-[var(--border)] bg-[var(--card)] flex flex-col shrink-0">
        <DatabaseTree
          selectedDb={selectedDB}
          selectedTable={selectedTable}
          onSelectDb={handleSelectDb}
          onSelectTable={handleSelectTable}
        />
      </div>

      {/* Main Content */}
      <div className="flex-1 flex flex-col min-w-0 bg-[var(--background)]">
        {/* Top Navigation Bar */}
        <div className="h-14 border-b border-[var(--border)] bg-[var(--card)] flex items-center px-4 justify-between shrink-0">
          <div className="flex items-center gap-2 text-sm">
            <div
              className="flex items-center gap-1 text-[var(--muted-foreground)] hover:text-[var(--foreground)] cursor-pointer"
              onClick={() => {
                setSelectedDB(null);
                setSelectedTable(null);
              }}
            >
              <Server size={16} />
              <span>127.0.0.1</span>
            </div>
            {selectedDB && (
              <>
                <ArrowRight
                  size={14}
                  className="text-[var(--muted-foreground)]"
                />
                <div
                  className="flex items-center gap-1 font-medium text-[var(--primary)] cursor-pointer"
                  onClick={() => setSelectedTable(null)}
                >
                  <DatabaseIcon size={16} />
                  <span>{selectedDB}</span>
                </div>
              </>
            )}
            {selectedTable && (
              <>
                <ArrowRight
                  size={14}
                  className="text-[var(--muted-foreground)]"
                />
                <div className="flex items-center gap-1 font-medium text-[var(--foreground)]">
                  <TableIcon size={16} />
                  <span>{selectedTable}</span>
                </div>
              </>
            )}
          </div>

          <div className="flex gap-2">{/* Global Actions can go here */}</div>
        </div>

        {/* Tab Bar */}
        <div className="px-4 pt-4 border-b border-[var(--border)] bg-[var(--card)]/50 backdrop-blur-sm shrink-0">
          <div className="flex items-center gap-1 overflow-x-auto no-scrollbar">
            {selectedTable ? (
              // Table Context Tabs
              <>
                <Button
                  variant={activeTab === "browse" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("browse")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)] data-[variant=primary]:rounded-b-none"
                >
                  <TableIcon size={14} /> Browse
                </Button>
                <Button
                  variant={activeTab === "structure" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("structure")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Columns size={14} /> Structure
                </Button>
                <Button
                  variant={activeTab === "sql" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("sql")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Code2 size={14} /> SQL
                </Button>
                <Button
                  variant={activeTab === "insert" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("insert")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Plus size={14} /> Insert
                </Button>
                <Button
                  variant={activeTab === "operations" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("operations")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Settings size={14} /> Operations
                </Button>
              </>
            ) : selectedDB ? (
              // Database Context Tabs
              <>
                <Button
                  variant={activeTab === "structure" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("structure")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <TableIcon size={14} /> Tables
                </Button>
                <Button
                  variant={activeTab === "sql" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("sql")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Code2 size={14} /> SQL
                </Button>
                <Button
                  variant={activeTab === "snapshots" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("snapshots")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <HardDrive size={14} /> Snapshots
                </Button>
                <Button
                  variant={activeTab === "export" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("export")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <FileUp size={14} /> Export
                </Button>
                <Button
                  variant={activeTab === "import" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("import")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <FileDown size={14} /> Import
                </Button>
              </>
            ) : (
              // Server Context Tabs
              <>
                <Button
                  variant={activeTab === "databases" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("databases")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <DatabaseIcon size={14} /> Databases
                </Button>
                <Button
                  variant={activeTab === "sql" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("sql")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Code2 size={14} /> SQL
                </Button>
              </>
            )}
          </div>
        </div>

        {/* Workspace Content */}
        <div className="flex-1 overflow-auto p-4 bg-[var(--background)]">
          {/* Context: Table, Tab: Browse */}
          {activeTab === "browse" && selectedTable && (
            <div className="space-y-4">
              {loadingData ? (
                <div className="flex justify-center py-10">
                  <RefreshCw className="animate-spin text-[var(--muted-foreground)]" />
                </div>
              ) : tableData ? (
                <div className="border border-[var(--border)] rounded-md overflow-hidden bg-[var(--card)]">
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm text-left">
                      <thead className="text-xs text-[var(--muted-foreground)] uppercase bg-[var(--muted)]/50 border-b border-[var(--border)]">
                        <tr>
                          <th className="px-4 py-3 font-medium">Actions</th>
                          {tableData.columns.map((col) => (
                            <th
                              key={col.name}
                              className="px-4 py-3 font-medium whitespace-nowrap"
                            >
                              {col.name}
                              <span className="ml-1 text-[10px] text-[var(--muted-foreground)] normal-case">
                                {col.type}
                              </span>
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-[var(--border)]">
                        {tableData.rows.map((row, i) => (
                          <tr
                            key={i}
                            className="hover:bg-[var(--muted)]/30 group"
                          >
                            <td className="px-4 py-2 w-20">
                              <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                <button
                                  className="p-1 hover:text-blue-400"
                                  onClick={() => startEdit(row)}
                                >
                                  <Code2 size={14} />
                                </button>
                                <button
                                  className="p-1 hover:text-red-400 disabled:opacity-30"
                                  disabled={!pkCol}
                                  onClick={() => handleDeleteRow(row)}
                                  title={
                                    !pkCol
                                      ? "No Primary Key found"
                                      : "Delete Row"
                                  }
                                >
                                  <Trash2 size={14} />
                                </button>
                              </div>
                            </td>
                            {tableData.columns.map((col) => (
                              <td
                                key={col.name}
                                className="px-4 py-2 whitespace-nowrap max-w-[300px] truncate"
                              >
                                <span
                                  className={cn(
                                    row[col.name] === null &&
                                      "text-[var(--muted-foreground)] italic"
                                  )}
                                >
                                  {row[col.name] === null
                                    ? "NULL"
                                    : String(row[col.name])}
                                </span>
                              </td>
                            ))}
                          </tr>
                        ))}
                        {tableData.rows.length === 0 && (
                          <tr>
                            <td
                              colSpan={tableData.columns.length + 1}
                              className="text-center py-8 text-[var(--muted-foreground)]"
                            >
                              No rows found
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>
                  {/* Pagination */}
                  <div className="px-4 py-3 border-t border-[var(--border)] flex items-center justify-between text-xs text-[var(--muted-foreground)]">
                    <span>Showing {tableData.rows.length} rows</span>
                    <div className="flex gap-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={page <= 1}
                        onClick={() => setPage((p) => p - 1)}
                      >
                        Previous
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={page >= tableData.total_pages}
                        onClick={() => setPage((p) => p + 1)}
                      >
                        Next
                      </Button>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="text-center py-12 text-[var(--muted-foreground)]">
                  Select a table to browse
                </div>
              )}
            </div>
          )}

          {/* Context: Table, Tab: Structure */}
          {activeTab === "structure" && selectedTable && tableSchema && (
            <div className="border border-[var(--border)] rounded-md overflow-hidden bg-[var(--card)]">
              <table className="w-full text-sm text-left">
                <thead className="text-xs text-[var(--muted-foreground)] uppercase bg-[var(--muted)]/50 border-b border-[var(--border)]">
                  <tr>
                    <th className="px-4 py-3 font-medium">Name</th>
                    <th className="px-4 py-3 font-medium">Type</th>
                    <th className="px-4 py-3 font-medium">Null</th>
                    <th className="px-4 py-3 font-medium">Default</th>
                    <th className="px-4 py-3 font-medium">Key</th>
                    <th className="px-4 py-3 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border)]">
                  {tableSchema.map((col) => (
                    <tr key={col.name} className="hover:bg-[var(--muted)]/30">
                      <td className="px-4 py-2 font-medium">{col.name}</td>
                      <td className="px-4 py-2 font-mono text-xs">
                        {col.type}
                      </td>
                      <td className="px-4 py-2">
                        {col.nullable ? "Yes" : "No"}
                      </td>
                      <td className="px-4 py-2 text-[var(--muted-foreground)]">
                        {col.default || "NULL"}
                      </td>
                      <td className="px-4 py-2">
                        <Badge variant="secondary" className="text-[10px]">
                          {col.key}
                        </Badge>
                      </td>
                      <td className="px-4 py-2">
                        <div className="flex gap-2">
                          <button className="text-[var(--muted-foreground)] hover:text-[var(--primary)] text-xs underline">
                            Change
                          </button>
                          <button className="text-[var(--muted-foreground)] hover:text-red-400 text-xs underline">
                            Drop
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Context: All, Tab: SQL */}
          {activeTab === "sql" && (
            <div className="h-full flex flex-col">
              <SQLConsole database={selectedDB || null} />
            </div>
          )}

          {/* Context: DB, Tab: Snapshots */}
          {activeTab === "snapshots" && selectedDB && (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              <Card
                className="border-dashed border-2 flex items-center justify-center p-6 cursor-pointer hover:border-[var(--primary)] transition-colors"
                onClick={() => createSnapshotMutation.mutate(selectedDB)}
              >
                <div className="text-center">
                  <div className="mx-auto w-12 h-12 bg-[var(--muted)] rounded-full flex items-center justify-center mb-3">
                    <Camera className="text-[var(--muted-foreground)]" />
                  </div>
                  <p className="font-medium">Create Snapshot</p>
                </div>
              </Card>
              {currentSnapshotList.map((snap) => (
                <Card key={snap.id}>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-base truncate">
                      {snap.filename}
                    </CardTitle>
                    <CardDescription>
                      {formatBytes(snap.size)} • {formatDate(snap.created_at)}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="flex gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="w-full"
                      onClick={() =>
                        window.open(
                          `${
                            import.meta.env.VITE_API_URL ||
                            "http://localhost:2025"
                          }/api/db/snapshots/download?id=${snap.filename}`,
                          "_blank"
                        )
                      }
                    >
                      <FileDown size={14} className="mr-2" /> Download
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      className="w-full"
                      onClick={() =>
                        restoreSnapshotMutation.mutate(snap.filename)
                      }
                    >
                      <RotateCcw size={14} className="mr-2" /> Restore
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      className="w-full"
                      onClick={() =>
                        deleteSnapshotMutation.mutate(snap.filename)
                      }
                    >
                      <Trash2 size={14} className="mr-2" /> Delete
                    </Button>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {/* Context: DB, Tab: Tables (Structure placeholder for DB level) */}
          {activeTab === "structure" && selectedDB && !selectedTable && (
            <div className="text-center py-12 text-[var(--muted-foreground)]">
              <TableIcon size={48} className="mx-auto mb-4 opacity-50" />
              <p>
                Select a table from the sidebar to view its structure or data.
              </p>
            </div>
          )}

          {/* Context: Insert / Edit */}
          {(activeTab === "insert" || activeTab === "edit") &&
            selectedTable &&
            tableSchema && (
              <div className="max-w-4xl mx-auto">
                <Card>
                  <CardHeader>
                    <CardTitle>
                      {activeTab === "edit" ? "Edit Row" : "Insert Row"}
                    </CardTitle>
                    <CardDescription>
                      {activeTab === "edit"
                        ? `Editing row in table ${selectedTable}`
                        : `Insert new row into ${selectedTable}`}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <DataForm
                      columns={tableSchema}
                      initialData={
                        activeTab === "edit"
                          ? editingRow || undefined
                          : undefined
                      }
                      onSubmit={handleSaveRow}
                      isLoading={executeQueryMutation.isPending}
                    />
                  </CardContent>
                </Card>
              </div>
            )}

          {/* Context: Import */}
          {activeTab === "import" && selectedDB && (
            <div className="max-w-2xl mx-auto mt-8">
              <Card>
                <CardHeader>
                  <CardTitle>Import Database</CardTitle>
                  <CardDescription>
                    Upload a .sql file to import into{" "}
                    <strong>{selectedDB}</strong>. This will execute the SQL
                    commands in the file against the selected database.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="flex flex-col gap-4">
                    <div className="border-2 border-dashed border-[var(--border)] rounded-lg p-8 text-center hover:bg-[var(--muted)]/20 transition-colors">
                      <FileUp className="mx-auto h-12 w-12 text-[var(--muted-foreground)] mb-4" />
                      <p className="text-sm text-[var(--muted-foreground)] mb-4">
                        Click to select or drag and drop a SQL file here
                      </p>
                      <input
                        type="file"
                        accept=".sql"
                        className="hidden"
                        ref={fileInputRef}
                        onChange={handleImport}
                        disabled={importDatabaseMutation.isPending}
                      />
                      <Button
                        onClick={() => fileInputRef.current?.click()}
                        variant="secondary"
                        loading={importDatabaseMutation.isPending}
                      >
                        Select SQL File
                      </Button>
                    </div>
                    <div className="text-xs text-[var(--muted-foreground)] text-center">
                      Supported formats: .sql, .sql.gz (uncompressed only for
                      now)
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>
          )}

          {/* Placeholders */}
          {["export", "operations", "databases"].includes(activeTab) && (
            <div className="flex flex-col items-center justify-center h-full text-[var(--muted-foreground)] opacity-50">
              <HardDrive size={48} className="mb-4" />
              <p>Feature coming soon...</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
