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
  Search,
  Zap,
} from "lucide-react";
import { formatBytes, formatDate } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/Card";
import { Label } from "@/components/ui/Label";
import { Input } from "@/components/ui/Input";
import { Badge } from "@/components/ui/Badge";
import { SQLConsole } from "@/components/database/SQLConsole";
// @ts-ignore
import { DatabaseTree } from "@/components/database/DatabaseTree";
import { DataForm } from "@/components/database/DataForm";
import { TableCreator } from "@/components/database/TableCreator";
import {
  useTableData,
  useTableColumns,
  useSnapshots,
  useDatabases,
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
  const [insertFormKey, setInsertFormKey] = useState(0);
  const [searchCriteria, setSearchCriteria] = useState<
    Record<string, { value: string; operator: string }>
  >({});
  const [searchResults, setSearchResults] = useState<any>(null);
  const [isSearching, setIsSearching] = useState(false);

  const fileInputRef = useRef<HTMLInputElement>(null);

  // Queries
  const { data: tableData, isLoading: loadingData } = useTableData(
    selectedDB,
    selectedTable,
    page
  );

  const { data: databases } = useDatabases();
  const { data: tableSchema } = useTableColumns(selectedDB, selectedTable);
  const { data: snapshots } = useSnapshots();

  // Helpers
  const pkCol = tableSchema?.find((c) => c.key === "PRI")?.name;

  const escapeValue = (val: any) => {
    if (val === null) return "NULL";
    if (typeof val === "number") return val;
    return `'${String(val).replace(/'/g, "\\'")}'`;
  };

  // Helper to safely get value from row (handles case mismatch)
  const getValue = (row: Record<string, any>, colName: string) => {
    if (!colName) return undefined;
    if (row[colName] !== undefined) return row[colName];
    // Try lowercase key match
    const lowerCol = colName.toLowerCase();
    const key = Object.keys(row).find((k) => k.toLowerCase() === lowerCol);
    if (key && row[key] !== undefined) return row[key];
    return undefined;
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

  const handleTruncateTable = () => {
    if (!selectedDB || !selectedTable) return;
    if (
      !confirm(
        `Are you sure you want to TRUNCATE table "${selectedTable}"? This will delete ALL data!`
      )
    )
      return;

    executeQueryMutation.mutate({
      database: selectedDB,
      query: `TRUNCATE TABLE \`${selectedTable}\``,
    });
  };

  const handleDropTable = () => {
    if (!selectedDB || !selectedTable) return;
    if (
      !confirm(
        `Are you sure you want to DROP table "${selectedTable}"? This action is IRREVERSIBLE!`
      )
    )
      return;

    executeQueryMutation.mutate(
      {
        database: selectedDB,
        query: `DROP TABLE \`${selectedTable}\``,
      },
      {
        onSuccess: () => {
          setSelectedTable(null);
        },
      }
    );
  };

  const handleDropDatabase = () => {
    if (!selectedDB) return;
    if (
      !confirm(
        `Are you sure you want to DROP database "${selectedDB}"? This action is IRREVERSIBLE!`
      )
    )
      return;

    executeQueryMutation.mutate(
      {
        database: selectedDB,
        query: `DROP DATABASE \`${selectedDB}\``,
      },
      {
        onSuccess: () => {
          setSelectedDB(null);
          setSelectedTable(null);
        },
      }
    );
  };

  const handleCreateDatabase = () => {
    const name = prompt("Enter database name:");
    if (!name) return;

    executeQueryMutation.mutate({
      database: "information_schema", // Connecting to any DB to run CREATE
      query: `CREATE DATABASE \`${name}\``,
    });
  };

  const handleDeleteRow = (row: Record<string, any>) => {
    if (!selectedDB || !selectedTable || !pkCol) return;

    if (!confirm("Are you sure you want to delete this row?")) return;

    const query = `DELETE FROM \`${selectedTable}\` WHERE \`${pkCol}\` = ${escapeValue(
      row[pkCol]
    )} LIMIT 1`;
    executeQueryMutation.mutate({ database: selectedDB, query });
  };

  const handleSaveRow = (
    data: Record<string, any>,
    mode: "save" | "save_and_add" = "save"
  ) => {
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
          if (mode === "save_and_add") {
            setInsertFormKey((k) => k + 1);
          } else {
            setActiveTab("browse");
            setEditingRow(null);
          }
        },
      }
    );
  };

  const handleSearch = () => {
    if (!selectedDB || !selectedTable || !tableSchema) return;

    const clauses: string[] = [];
    Object.entries(searchCriteria).forEach(([col, criteria]) => {
      if (
        criteria.value === "" &&
        !["IS NULL", "IS NOT NULL"].includes(criteria.operator)
      )
        return;

      let clause = `\`${col}\` `;
      const val = escapeValue(criteria.value);

      switch (criteria.operator) {
        case "=":
        case "!=":
        case ">":
        case "<":
        case ">=":
        case "<=":
          clause += `${criteria.operator} ${val}`;
          break;
        case "LIKE":
          clause += `LIKE ${val}`;
          break;
        case "LIKE %...%":
          clause += `LIKE '%${String(criteria.value).replace(/'/g, "\\'")}%'`;
          break;
        case "IS NULL":
          clause += `IS NULL`;
          break;
        case "IS NOT NULL":
          clause += `IS NOT NULL`;
          break;
      }
      clauses.push(clause);
    });

    if (clauses.length === 0) {
      setSearchResults(null);
      setActiveTab("browse");
      return;
    }

    const query = `SELECT * FROM \`${selectedTable}\` WHERE ${clauses.join(
      " AND "
    )} LIMIT 1000`;

    setIsSearching(true);
    executeQueryMutation.mutate(
      { database: selectedDB, query },
      {
        onSuccess: (data) => {
          // Enrich query results with schema info for Browse tab
          const enriched = {
            ...data,
            columns: tableSchema.map((col) => ({
              name: col.name,
              type: col.type,
            })),
            total: data.rowCount,
            total_pages: 1, // Simple result set for now
          };
          setSearchResults(enriched);
          setActiveTab("browse");
          setIsSearching(false);
        },
        onError: () => {
          setIsSearching(false);
        },
      }
    );
  };

  const clearSearch = () => {
    setSearchCriteria({});
    setSearchResults(null);
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

  const handleCreateTable = (query: string) => {
    if (!selectedDB) return;

    executeQueryMutation.mutate(
      { database: selectedDB, query },
      {
        onSuccess: () => {
          // Refresh tables (invalidate queries)
          // Since we rely on react-query invalidation in hook (likely),
          // we just need to switch view.
          // Ideally we select the new table, but let's just go to structure view of DB for now.
          setActiveTab("structure");
        },
      }
    );
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
          onCreateTable={(db) => {
            setSelectedDB(db);
            setSelectedTable(null);
            setActiveTab("create-table");
          }}
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
                  variant={activeTab === "search" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("search")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Search size={14} /> Search
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
                <Button
                  variant={activeTab === "operations" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("operations")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Settings size={14} /> Operations
                </Button>
                <Button
                  variant={activeTab === "triggers" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("triggers")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Zap size={14} /> Triggers
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
                <Button
                  variant={activeTab === "create-table" ? "primary" : "ghost"}
                  size="sm"
                  onClick={() => setActiveTab("create-table")}
                  className="gap-2 rounded-b-none border-b-2 border-transparent data-[variant=primary]:border-[var(--primary)]"
                >
                  <Plus size={14} /> Create Table
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
              {loadingData || isSearching ? (
                <div className="flex justify-center py-10">
                  <RefreshCw className="animate-spin text-[var(--muted-foreground)]" />
                </div>
              ) : !(searchResults || tableData) ? (
                <div className="text-center py-12 text-[var(--muted-foreground)]">
                  Select a table to browse
                </div>
              ) : (
                <div className="border border-[var(--border)] rounded-md overflow-hidden bg-[var(--card)]">
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm text-left">
                      <thead className="text-xs text-[var(--muted-foreground)] uppercase bg-[var(--muted)]/50 border-b border-[var(--border)]">
                        <tr>
                          <th className="px-4 py-3 font-medium">Actions</th>
                          {(searchResults || tableData).columns?.map(
                            (col: any) => (
                              <th
                                key={typeof col === "string" ? col : col.name}
                                className="px-4 py-3 font-medium whitespace-nowrap"
                              >
                                {typeof col === "string" ? col : col.name}
                                <span className="ml-1 text-[10px] text-[var(--muted-foreground)] normal-case">
                                  {typeof col === "string" ? "" : col.type}
                                </span>
                              </th>
                            )
                          )}
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-[var(--border)]">
                        {((searchResults || tableData).rows || []).map(
                          (row: any, i: number) => (
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
                              {(searchResults || tableData).columns?.map(
                                (col: any) => {
                                  const colName =
                                    typeof col === "string" ? col : col.name;
                                  return (
                                    <td
                                      key={colName}
                                      className="px-4 py-2 whitespace-nowrap max-w-[300px] truncate"
                                    >
                                      <span>
                                        {(() => {
                                          const val = getValue(row, colName);
                                          if (val === null) return "NULL";
                                          if (val === undefined)
                                            return (
                                              <span className="text-red-400 text-[10px]">
                                                (missing)
                                              </span>
                                            );
                                          return String(val);
                                        })()}
                                      </span>
                                    </td>
                                  );
                                }
                              )}
                            </tr>
                          )
                        )}
                        {((searchResults || tableData).rows || []).length ===
                          0 && (
                          <tr>
                            <td
                              colSpan={
                                ((searchResults || tableData).columns || [])
                                  .length + 1
                              }
                              className="px-4 py-8 text-center text-[var(--muted-foreground)] italic"
                            >
                              No data found
                            </td>
                          </tr>
                        )}

                        {/* Pagination / Status */}
                        <tr>
                          <td
                            colSpan={
                              ((searchResults || tableData).columns || [])
                                .length + 1
                            }
                            className="px-4 py-2 text-xs text-[var(--muted-foreground)] border-t border-[var(--border)]"
                          >
                            <div className="flex justify-between items-center">
                              <span>
                                Showing{" "}
                                {
                                  ((searchResults || tableData).rows || [])
                                    .length
                                }{" "}
                                rows
                                {searchResults && (
                                  <span className="ml-2 text-blue-400 font-medium whitespace-nowrap">
                                    (Filtered results)
                                    <button
                                      onClick={clearSearch}
                                      className="ml-2 hover:underline"
                                    >
                                      Clear
                                    </button>
                                  </span>
                                )}
                              </span>
                              <div className="flex gap-2">
                                <Button
                                  variant="secondary"
                                  size="sm"
                                  disabled={searchResults || page <= 1}
                                  onClick={() => setPage((p) => p - 1)}
                                >
                                  Previous
                                </Button>
                                <Button
                                  variant="secondary"
                                  size="sm"
                                  disabled={
                                    searchResults ||
                                    page >=
                                      (searchResults || tableData).total_pages
                                  }
                                  onClick={() => setPage((p) => p + 1)}
                                >
                                  Next
                                </Button>
                              </div>
                            </div>
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Context: Table, Tab: Insert */}
          {activeTab === "insert" && selectedTable && tableSchema && (
            <div className="max-w-6xl mx-auto mt-8">
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    <Plus size={18} /> Insert Row
                  </CardTitle>
                  <CardDescription>
                    Insert data into table <strong>{selectedTable}</strong>.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <DataForm
                    key={`insert-${insertFormKey}`}
                    columns={tableSchema}
                    onSubmit={handleSaveRow}
                    isLoading={executeQueryMutation.isPending}
                  />
                </CardContent>
              </Card>
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
                  {(tableSchema || []).map((col) => (
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
                className="col-span-full w-full border-dashed border-2 flex items-center justify-center p-6 cursor-pointer hover:border-[var(--primary)] transition-colors"
                onClick={() => createSnapshotMutation.mutate(selectedDB)}
              >
                <div className="text-center">
                  <div className="mx-auto w-12 h-12 bg-[var(--muted)] rounded-full flex items-center justify-center mb-3">
                    <Camera className="text-[var(--muted-foreground)]" />
                  </div>
                  <p className="font-medium">Create Snapshot</p>
                </div>
              </Card>
              {(currentSnapshotList || []).map((snap) => (
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

          {/* Context: Edit */}
          {activeTab === "edit" && selectedTable && tableSchema && (
            <div className="max-w-4xl mx-auto">
              <Card>
                <CardHeader>
                  <CardTitle>Edit Row</CardTitle>
                  <CardDescription>
                    Editing row in table <strong>{selectedTable}</strong>
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <DataForm
                    columns={tableSchema}
                    initialData={editingRow || undefined}
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

          {/* Context: Create Table */}
          {activeTab === "create-table" && selectedDB && (
            <div className="p-6">
              <TableCreator
                database={selectedDB}
                onCancel={() => setActiveTab("structure")}
                onSave={handleCreateTable}
                isLoading={executeQueryMutation.isPending}
              />
            </div>
          )}

          {/* Context: Operations */}
          {activeTab === "operations" && selectedDB && (
            <div className="max-w-2xl mx-auto mt-8 flex flex-col gap-6">
              {/* Context: Table, Tab: Operations */}
              {activeTab === "operations" && selectedTable && (
                <div className="space-y-6 max-w-2xl">
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-lg flex items-center gap-2">
                        <Settings size={18} /> Table Options
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="space-y-2">
                        <Label>Rename table to:</Label>
                        <div className="flex gap-2">
                          <Input
                            defaultValue={selectedTable}
                            id="rename-table-input"
                          />
                          <Button
                            onClick={() => {
                              const newName = (
                                document.getElementById(
                                  "rename-table-input"
                                ) as HTMLInputElement
                              ).value;
                              if (newName && newName !== selectedTable) {
                                executeQueryMutation.mutate(
                                  {
                                    database: selectedDB!,
                                    query: `RENAME TABLE \`${selectedTable}\` TO \`${newName}\``,
                                  },
                                  {
                                    onSuccess: () => {
                                      setSelectedTable(newName);
                                      // Refresh tables implicitly
                                    },
                                  }
                                );
                              }
                            }}
                          >
                            Go
                          </Button>
                        </div>
                      </div>

                      <div className="space-y-2 pt-4 border-t border-[var(--border)]">
                        <Label>Copy table to (database.table):</Label>
                        <div className="flex gap-2">
                          <Input
                            placeholder={`${selectedTable}_copy`}
                            id="copy-table-input"
                          />
                          <Button
                            variant="secondary"
                            onClick={() => {
                              const newName = (
                                document.getElementById(
                                  "copy-table-input"
                                ) as HTMLInputElement
                              ).value;
                              if (newName) {
                                executeQueryMutation.mutate(
                                  {
                                    database: selectedDB!,
                                    query: `CREATE TABLE \`${newName}\` LIKE \`${selectedTable}\`; INSERT INTO \`${newName}\` SELECT * FROM \`${selectedTable}\`;`,
                                  },
                                  {
                                    onSuccess: () => alert("Table copied!"),
                                  }
                                );
                              }
                            }}
                          >
                            Copy
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>

                  <Card className="border-red-200 dark:border-red-900/20">
                    <CardHeader>
                      <CardTitle className="text-lg text-red-500 flex items-center gap-2">
                        <Trash2 size={18} /> Table Maintenance
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="flex flex-col gap-2">
                        <div className="flex justify-between items-center p-3 rounded-md bg-red-50 dark:bg-red-900/10">
                          <div className="text-sm font-medium">
                            Empty the table (TRUNCATE)
                          </div>
                          <Button
                            variant="danger"
                            size="sm"
                            onClick={handleTruncateTable}
                          >
                            Truncate
                          </Button>
                        </div>
                        <div className="flex justify-between items-center p-3 rounded-md bg-red-50 dark:bg-red-900/10">
                          <div className="text-sm font-medium">
                            Delete the table (DROP)
                          </div>
                          <Button
                            variant="danger"
                            size="sm"
                            onClick={handleDropTable}
                          >
                            Drop
                          </Button>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                </div>
              )}
              {/* Database Operations */}
              {!selectedTable && (
                <Card className="border-red-200 dark:border-red-900/50">
                  <CardHeader>
                    <CardTitle className="text-red-600 dark:text-red-400">
                      Database Operations
                    </CardTitle>
                    <CardDescription>
                      Danger zone for database <strong>{selectedDB}</strong>.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button
                      variant="danger"
                      className="w-full"
                      onClick={handleDropDatabase}
                      loading={executeQueryMutation.isPending}
                    >
                      <Trash2 size={16} className="mr-2" /> Drop Database
                    </Button>
                  </CardContent>
                </Card>
              )}

              {/* Table Operations */}
              {selectedTable && (
                <Card>
                  <CardHeader>
                    <CardTitle>Table Operations</CardTitle>
                    <CardDescription>
                      Manage table <strong>{selectedTable}</strong>.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="flex flex-col gap-4">
                    <div className="border rounded-md p-4 flex items-center justify-between">
                      <div>
                        <h4 className="font-medium text-sm">Truncate Table</h4>
                        <p className="text-xs text-[var(--muted-foreground)]">
                          Delete all rows but keep structure.
                        </p>
                      </div>
                      <Button
                        variant="secondary"
                        onClick={handleTruncateTable}
                        loading={executeQueryMutation.isPending}
                      >
                        Truncate
                      </Button>
                    </div>
                    <div className="border border-red-200 dark:border-red-900/50 rounded-md p-4 flex items-center justify-between bg-red-50/50 dark:bg-red-900/10">
                      <div>
                        <h4 className="font-medium text-sm text-red-600 dark:text-red-400">
                          Drop Table
                        </h4>
                        <p className="text-xs text-red-600/70 dark:text-red-400/70">
                          Delete the table and all its data.
                        </p>
                      </div>
                      <Button
                        variant="danger"
                        onClick={handleDropTable}
                        loading={executeQueryMutation.isPending}
                      >
                        Drop Table
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              )}
            </div>
          )}

          {/* Context: Databases Management */}
          {activeTab === "databases" && (
            <div className="max-w-4xl mx-auto mt-8">
              <div className="flex justify-between items-center mb-6">
                <h2 className="text-xl font-semibold">Databases</h2>
                <Button onClick={handleCreateDatabase}>
                  <Plus size={16} className="mr-2" /> Create Database
                </Button>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {(databases || []).map((db) => (
                  <Card
                    key={db.name}
                    className="hover:border-[var(--primary)] transition-colors"
                  >
                    <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                      <CardTitle className="text-base font-medium">
                        {db.name}
                      </CardTitle>
                      <DatabaseIcon
                        size={16}
                        className="text-[var(--muted-foreground)]"
                      />
                    </CardHeader>
                    <CardContent>
                      <div className="text-xs text-[var(--muted-foreground)] mb-4">
                        {db.tables ?? 0} tables
                      </div>
                      <div className="flex gap-2">
                        <Button
                          variant="secondary"
                          size="sm"
                          className="w-full"
                          onClick={() => setSelectedDB(db.name)}
                        >
                          Manage
                        </Button>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => {
                            // Dirty hack to set selectedDB for the handler, or just call mutate directly
                            if (confirm(`Drop database ${db.name}?`)) {
                              executeQueryMutation.mutate({
                                database: db.name,
                                query: `DROP DATABASE \`${db.name}\``,
                              });
                            }
                          }}
                        >
                          <Trash2 size={14} />
                        </Button>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>
          )}

          {/* Placeholders */}
          {/* Context: Table, Tab: Search */}
          {activeTab === "search" && selectedTable && tableSchema && (
            <div className="max-w-6xl mx-auto mt-8">
              <Card>
                <CardHeader>
                  <CardTitle className="text-lg flex items-center gap-2">
                    <Search size={18} /> Search Table
                  </CardTitle>
                  <CardDescription>
                    Filter results from <strong>{selectedTable}</strong> using
                    column criteria.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="border border-[var(--border)] rounded-md overflow-hidden mb-6">
                    <table className="w-full text-sm text-left">
                      <thead className="bg-[var(--muted)]/50 border-b border-[var(--border)] text-xs uppercase text-[var(--muted-foreground)]">
                        <tr>
                          <th className="px-4 py-3 font-medium w-1/4">
                            Column
                          </th>
                          <th className="px-4 py-3 font-medium w-1/6">Type</th>
                          <th className="px-4 py-3 font-medium w-1/4">
                            Operator
                          </th>
                          <th className="px-4 py-3 font-medium">Value</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-[var(--border)] bg-[var(--card)]">
                        {tableSchema.map((col) => (
                          <tr
                            key={col.name}
                            className="hover:bg-[var(--muted)]/20"
                          >
                            <td className="px-4 py-3">
                              <div className="font-medium">{col.name}</div>
                            </td>
                            <td className="px-4 py-3 text-xs font-mono text-[var(--muted-foreground)]">
                              {col.type}
                            </td>
                            <td className="px-4 py-3">
                              <select
                                className="w-full h-8 px-2 bg-[var(--card)] border border-[var(--border)] rounded text-sm focus:outline-none focus:ring-1 focus:ring-[var(--primary)] text-[var(--foreground)]"
                                value={
                                  searchCriteria[col.name]?.operator || "="
                                }
                                onChange={(e) =>
                                  setSearchCriteria((prev) => ({
                                    ...prev,
                                    [col.name]: {
                                      ...prev[col.name],
                                      operator: e.target.value,
                                    },
                                  }))
                                }
                              >
                                <option
                                  value="="
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  =
                                </option>
                                <option
                                  value="!="
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  !=
                                </option>
                                <option
                                  value="LIKE"
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  LIKE
                                </option>
                                <option
                                  value="LIKE %...%"
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  LIKE %...%
                                </option>
                                <option
                                  value=">"
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  &gt;
                                </option>
                                <option
                                  value="<"
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  &lt;
                                </option>
                                <option
                                  value=">="
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  &gt;=
                                </option>
                                <option
                                  value="<="
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  &lt;=
                                </option>
                                <option
                                  value="IS NULL"
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  IS NULL
                                </option>
                                <option
                                  value="IS NOT NULL"
                                  className="bg-[var(--card)] text-[var(--foreground)]"
                                >
                                  IS NOT NULL
                                </option>
                              </select>
                            </td>
                            <td className="px-4 py-3">
                              <Input
                                type={
                                  col.type.toLowerCase().includes("int")
                                    ? "number"
                                    : "text"
                                }
                                placeholder="Search value..."
                                className="h-8 text-sm"
                                disabled={searchCriteria[
                                  col.name
                                ]?.operator?.includes("NULL")}
                                value={searchCriteria[col.name]?.value || ""}
                                onChange={(e) =>
                                  setSearchCriteria((prev) => ({
                                    ...prev,
                                    [col.name]: {
                                      ...prev[col.name],
                                      value: e.target.value,
                                      operator: prev[col.name]?.operator || "=",
                                    },
                                  }))
                                }
                              />
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>

                  <div className="flex justify-end gap-3">
                    <Button
                      variant="ghost"
                      onClick={() => {
                        clearSearch();
                        setActiveTab("browse");
                      }}
                    >
                      Reset
                    </Button>
                    <Button
                      onClick={handleSearch}
                      loading={isSearching}
                      className="gap-2"
                    >
                      <Search size={16} /> Search
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>
          )}

          {/* Context: Other Tabs Placeholders */}
          {["export", "triggers"].includes(activeTab) && (
            <div className="text-center py-12 text-[var(--muted-foreground)]">
              {activeTab.charAt(0).toUpperCase() + activeTab.slice(1)} coming
              soon...
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
