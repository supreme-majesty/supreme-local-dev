/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useEffect } from "react";
import {
  Plus,
  RefreshCw,
  Copy,
  Trash2,
  Table,
  Globe,
  ShieldCheck,
  Search,
  Wrench,
  BarChart2,
  ExternalLink,
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/Card";
import {
  useRenameDatabaseMutation,
  useCloneDatabaseMutation,
  useDeleteDatabaseMutation,
  useExecuteQueryMutation,
  useCollations,
  useDatabaseSettings,
  useTables,
  useDBSearchQuery,
  useMaintenanceMutation,
} from "@/hooks/use-database";
import { useAppStore } from "@/stores/useAppStore";

interface DatabaseOperationsProps {
  database: string;
  onSelectDb: (db: string) => void;
  onSelectTable: (table: string) => void;
  onNavigateToRecord: (table: string, match: Record<string, unknown>) => void;
  onCreateTable: () => void;
}

export function DatabaseOperations({
  database,
  onSelectDb,
  onSelectTable,
  onNavigateToRecord,
  onCreateTable,
}: DatabaseOperationsProps) {
  const [newName, setNewName] = useState(database);
  const [copyName, setCopyName] = useState(`${database}_copy`);
  const [copyOption, setCopyOption] = useState<"structure" | "both" | "data">(
    "both",
  );
  const [createDb, setCreateDb] = useState(true);
  const [addDrop, setAddDrop] = useState(false);
  const [addConstraints, setAddConstraints] = useState(true);
  const [switchDb, setSwitchDb] = useState(true);
  const [selectedCollation, setSelectedCollation] = useState("");
  const [changeAllTables, setChangeAllTables] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");

  const addToast = (useAppStore as any)((s: any) => s.addToast);

  const renameMutation = useRenameDatabaseMutation();
  const cloneMutation = useCloneDatabaseMutation();
  const deleteMutation = useDeleteDatabaseMutation();
  const executeMutation = useExecuteQueryMutation();
  const maintenanceMutation = useMaintenanceMutation();

  const { data: collations, isLoading: loadingCollations } = useCollations();
  const { data: currentSettings } = useDatabaseSettings(database);
  const { data: tables } = useTables(database);
  const { data: searchResults, isLoading: isSearching } = useDBSearchQuery(
    database,
    searchQuery,
  );

  useEffect(() => {
    if (currentSettings?.collation) {
      setSelectedCollation(currentSettings.collation);
    }
  }, [currentSettings]);

  const handleRename = () => {
    if (!newName || newName === database) return;
    renameMutation.mutate(
      { oldName: database, newName },
      {
        onSuccess: () => onSelectDb(newName),
      },
    );
  };

  const handleCopy = () => {
    if (!copyName) return;
    cloneMutation.mutate(
      {
        source: database,
        target: copyName,
        mode: copyOption,
        create_db: createDb,
        add_drop: addDrop,
        add_auto_inc: true,
        add_constraints: addConstraints,
      },
      {
        onSuccess: () => {
          if (switchDb) {
            onSelectDb(copyName);
          }
        },
      },
    );
  };

  const handleMaintenance = (op: string) => {
    maintenanceMutation.mutate({ database, tables: [], operation: op });
  };

  const formatSize = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  // Calculate storage insights
  const tableStorage =
    tables
      ?.map((t) => ({
        name: t.name,
        size: t.size,
        rowCount: t.row_count,
      }))
      .sort((a, b) => b.size - a.size)
      .slice(0, 5) || [];

  const totalSize = tables?.reduce((acc, t) => acc + t.size, 0) || 0;

  const handleSetCollation = async (
    collationOverride?: string,
    allTablesOverride?: boolean,
  ) => {
    const targetCollation =
      collationOverride !== undefined ? collationOverride : selectedCollation;
    const targetAll =
      allTablesOverride !== undefined ? allTablesOverride : changeAllTables;

    if (!targetCollation) return;

    const collationObj = collations?.find((c) => c.name === targetCollation);
    if (!collationObj) return;

    try {
      // 1. Alter Database
      await executeMutation.mutateAsync({
        database,
        query: `ALTER DATABASE \`${database}\` CHARACTER SET ${collationObj.charset} COLLATE ${targetCollation}`,
      });

      // 2. Alter All Tables if checked
      if (targetAll && tables) {
        addToast({
          type: "info",
          title: "Updating Tables",
          description: `Updating collation for ${tables.length} tables...`,
        });

        for (const table of tables) {
          await executeMutation.mutateAsync({
            database,
            query: `ALTER TABLE \`${table.name}\` CONVERT TO CHARACTER SET ${collationObj.charset} COLLATE ${targetCollation}`,
          });
        }
      }

      addToast({
        type: "success",
        title: "Collation updated",
        description: `Database ${database} ${targetAll ? "and all tables " : ""}updated to ${targetCollation}`,
      });
    } catch (err: unknown) {
      const error = err as Error;
      addToast({
        type: "error",
        title: "Failed to update collation",
        description: error.message,
      });
    }
  };

  return (
    <div className="space-y-6 pb-12 mt-4 px-6 max-w-full overflow-x-hidden">
      {/* Global Database Search */}
      <Card className="border-blue-500/20 bg-blue-500/5 overflow-visible">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2 text-blue-600">
            <Search size={18} /> Global Database Search
          </CardTitle>
          <CardDescription>
            Search for data across all tables and columns in{" "}
            <strong>{database}</strong>
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-2">
            <div className="relative flex-1">
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Type to search (min 2 chars)..."
                className="pl-10"
              />
              <Search
                size={16}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]"
              />
            </div>
            <Button variant="outline" onClick={() => setSearchQuery("")}>
              Clear
            </Button>
          </div>

          {searchQuery.length >= 2 &&
            !isSearching &&
            (searchResults as any)?.results?.length > 0 && (
              <div className="mb-4 flex items-center gap-2 text-xs text-[var(--muted-foreground)] animate-in fade-in zoom-in duration-300">
                <span className="flex items-center gap-1 bg-emerald-500/10 text-emerald-600 px-2 py-0.5 rounded-full font-bold">
                  <ShieldCheck size={12} />
                  Found{" "}
                  {((searchResults as any).results as any[]).reduce(
                    (acc: number, r: { row_count: number }) => acc + r.row_count,
                    0,
                  )}{" "}
                  matches
                </span>
                <span>
                  across {(searchResults as any).results.length} tables in {database}
                </span>
              </div>
            )}

          {searchQuery.length >= 2 && (
            <div className="bg-[var(--background)] border border-[var(--border)] rounded-lg overflow-hidden animate-in fade-in slide-in-from-top-2">
              {isSearching ? (
                <div className="p-8 text-center text-sm text-[var(--muted-foreground)]">
                  <RefreshCw
                    size={24}
                    className="animate-spin mx-auto mb-2 opacity-50"
                  />
                  Scanning all tables...
                </div>
              ) : (searchResults as any)?.results?.length > 0 ? (
                <div className="max-h-[400px] overflow-y-auto">
                  {(searchResults as any).results.map((res: { table: string; row_count: number; matches: Record<string, unknown>[] }, i: number) => (
                    <div
                      key={i}
                      className="p-4 border-b border-[var(--border)] last:border-0 hover:bg-[var(--muted)]/20 transition-colors"
                    >
                      <div className="flex items-center justify-between mb-3">
                        <button
                          onClick={() => onSelectTable(res.table)}
                          className="font-bold text-sm text-blue-600 flex items-center gap-2 hover:underline cursor-pointer group"
                        >
                          <Table
                            size={14}
                            className="group-hover:scale-110 transition-transform"
                          />{" "}
                          {res.table}
                        </button>
                        <span className="text-[10px] uppercase tracking-wider font-bold bg-blue-500/10 text-blue-600 px-2 py-0.5 rounded-full">
                          {res.row_count} matches
                        </span>
                      </div>
                      <div className="space-y-2">
                        {res.matches
                          ?.slice(0, 3)
                          .map((match: Record<string, unknown>, j: number) => {
                            // Find columns that match the search query
                            const matchingCols = Object.entries(match).filter(
                              ([, val]) =>
                                String(val)
                                  .toLowerCase()
                                  .includes(searchQuery.toLowerCase()),
                            );

                            return (
                              <div
                                key={j}
                                onClick={() =>
                                  onNavigateToRecord(res.table, match)
                                }
                                className="group/match text-xs border-l-2 border-blue-500/30 bg-[var(--muted)]/30 rounded-r overflow-hidden hover:border-blue-500 transition-colors cursor-pointer"
                              >
                                <div className="flex items-start justify-between p-2">
                                  <div className="flex flex-wrap gap-x-4 gap-y-1 flex-1">
                                    {matchingCols.map(([col, val], k) => (
                                      <div
                                        key={k}
                                        className="flex gap-1.5 items-baseline"
                                      >
                                        <span className="text-[var(--muted-foreground)] font-mono text-[10px]">
                                          {col}:
                                        </span>
                                        <span className="text-[var(--foreground)] font-medium">
                                          {String(val)
                                            .split(
                                              new RegExp(
                                                `(${searchQuery})`,
                                                "gi",
                                              ),
                                            )
                                            .map((part, idx) =>
                                              part.toLowerCase() ===
                                              searchQuery.toLowerCase() ? (
                                                <mark
                                                  key={idx}
                                                  className="bg-yellow-500/30 text-[var(--foreground)] rounded px-0.5"
                                                >
                                                  {part}
                                                </mark>
                                              ) : (
                                                part
                                              ),
                                            )}
                                        </span>
                                      </div>
                                    ))}
                                    {Object.entries(match)
                                      .filter(
                                        ([col]) =>
                                          !matchingCols.find(
                                            ([mc]) => mc === col,
                                          ),
                                      )
                                      .slice(0, 2)
                                      .map(([col, val], k) => (
                                        <div
                                          key={k}
                                          className="flex gap-1.5 items-baseline opacity-40 group-hover/match:opacity-70 transition-opacity"
                                        >
                                          <span className="text-[var(--muted-foreground)] font-mono text-[10px]">
                                            {col}:
                                          </span>
                                          <span className="truncate max-w-[120px]">
                                            {String(val)}
                                          </span>
                                        </div>
                                      ))}
                                  </div>
                                  <div className="h-6 w-6 flex items-center justify-center opacity-0 group-hover/match:opacity-100 transition-opacity text-blue-500">
                                    <ExternalLink size={12} />
                                  </div>
                                </div>
                              </div>
                            );
                          })}
                        {res.row_count > 3 && (
                          <div
                            className="text-[10px] text-center text-[var(--muted-foreground)] pt-1 italic hover:text-blue-500 cursor-pointer"
                            onClick={() => onSelectTable(res.table)}
                          >
                            + {res.row_count - 3} more matches in this table...
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="p-8 text-center text-sm text-[var(--muted-foreground)]">
                  No matches found for "{searchQuery}"
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Management */}
        <div className="space-y-6 lg:col-span-1">
          {/* Create New Table */}
          <Card className="border-emerald-500/20 bg-emerald-500/5">
            <CardHeader className="pb-3">
              <CardTitle className="text-base flex items-center gap-2 text-emerald-600">
                <Plus size={18} /> New Table
              </CardTitle>
            </CardHeader>
            <CardContent>
              <Button
                onClick={onCreateTable}
                className="w-full bg-emerald-600 hover:bg-emerald-700"
              >
                <Table size={16} className="mr-2" /> Start Table Designer
              </Button>
            </CardContent>
          </Card>

          {/* Rename Database */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <RefreshCw size={16} className="text-blue-500" /> Rename
                Database
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Input
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="New database name..."
                className="h-9 text-sm"
              />
              <Button
                onClick={handleRename}
                disabled={
                  renameMutation.isPending || !newName || newName === database
                }
                className="w-full h-9"
              >
                {renameMutation.isPending ? "Renaming..." : "Apply"}
              </Button>
            </CardContent>
          </Card>

          {/* Danger Zone */}
          <Card className="border-red-500/20 bg-red-500/5">
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-bold flex items-center gap-2 text-red-600">
                <Trash2 size={16} /> Danger Zone
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="text-xs text-[var(--muted-foreground)] leading-relaxed">
                Dropping <strong>{database}</strong> will permanently delete all
                its tables and data.
              </div>
              <Button
                variant="danger"
                onClick={() => {
                  if (
                    confirm(
                      `Are you absolutely sure you want to DROP the entire database "${database}"? This cannot be undone.`,
                    )
                  ) {
                    deleteMutation.mutate(database);
                  }
                }}
                disabled={deleteMutation.isPending}
                className="w-full h-9 bg-red-600 hover:bg-red-700"
              >
                {deleteMutation.isPending ? "Dropping..." : "Drop Database"}
              </Button>
            </CardContent>
          </Card>
        </div>

        {/* Center Column: Operations */}
        <div className="space-y-6 lg:col-span-1">
          {/* Copy Database */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <Copy size={16} className="text-purple-500" /> Copy Database to
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Input
                value={copyName}
                onChange={(e) => setCopyName(e.target.value)}
                placeholder="Copy to..."
                className="h-9 text-sm"
              />

              <div className="grid grid-cols-1 gap-2 p-2 bg-[var(--muted)]/30 rounded-lg">
                <label className="flex items-center gap-2 text-xs cursor-pointer group">
                  <input
                    type="radio"
                    checked={copyOption === "both"}
                    onChange={() => setCopyOption("both")}
                    className="accent-[var(--primary)]"
                  />
                  <span>Structure and data</span>
                </label>
                <label className="flex items-center gap-2 text-xs cursor-pointer group">
                  <input
                    type="radio"
                    checked={copyOption === "structure"}
                    onChange={() => setCopyOption("structure")}
                    className="accent-[var(--primary)]"
                  />
                  <span>Structure only</span>
                </label>
                <label className="flex items-center gap-2 text-xs cursor-pointer group">
                  <input
                    type="radio"
                    checked={copyOption === "data"}
                    onChange={() => setCopyOption("data")}
                    className="accent-[var(--primary)]"
                  />
                  <span>Data only</span>
                </label>
              </div>

              <div className="space-y-1 pt-2">
                {[
                  {
                    label: "Create database",
                    state: createDb,
                    setter: setCreateDb,
                  },
                  {
                    label: "Add DROP TABLE",
                    state: addDrop,
                    setter: setAddDrop,
                  },
                  {
                    label: "Add constraints",
                    state: addConstraints,
                    setter: setAddConstraints,
                  },
                  {
                    label: "Switch to copy",
                    state: switchDb,
                    setter: setSwitchDb,
                  },
                ].map((opt) => (
                  <label
                    key={opt.label}
                    className="flex items-center gap-2 text-[10px] uppercase font-bold tracking-wider text-[var(--muted-foreground)] cursor-pointer hover:text-[var(--primary)] transition-colors"
                  >
                    <input
                      type="checkbox"
                      checked={opt.state}
                      onChange={(e) => opt.setter(e.target.checked)}
                      className="accent-[var(--primary)]"
                    />
                    {opt.label}
                  </label>
                ))}
              </div>

              <Button
                onClick={handleCopy}
                disabled={cloneMutation.isPending || !copyName}
                variant="secondary"
                className="w-full h-9"
              >
                {cloneMutation.isPending ? "Copying..." : "Run Copy"}
              </Button>
            </CardContent>
          </Card>

          {/* Collation */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <Globe size={16} className="text-orange-500" /> Global Collation
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="relative">
                <select
                  value={selectedCollation}
                  onChange={(e) => {
                    setSelectedCollation(e.target.value);
                    handleSetCollation(e.target.value);
                  }}
                  className="w-full bg-[var(--background)] border border-[var(--border)] rounded-md h-9 px-3 text-xs focus:outline-none focus:ring-2 focus:ring-[var(--primary)]/50 appearance-none pr-8 cursor-pointer hover:border-[var(--primary)]/50 transition-colors"
                  disabled={loadingCollations || executeMutation.isPending}
                >
                  <option value="">
                    {loadingCollations ? "Loading..." : "Select collation..."}
                  </option>
                  {collations?.map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.name}
                    </option>
                  ))}
                </select>
                <div className="absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none opacity-50">
                  {executeMutation.isPending ? (
                    <RefreshCw
                      size={12}
                      className="animate-spin text-[var(--primary)]"
                    />
                  ) : (
                    <div className="border-l border-t border-[var(--foreground)] w-1.5 h-1.5 rotate-[225deg] mt-[-3px]" />
                  )}
                </div>
              </div>

              <label className="flex items-center gap-2 text-[10px] uppercase font-bold tracking-wider text-[var(--muted-foreground)] cursor-pointer group">
                <input
                  type="checkbox"
                  checked={changeAllTables}
                  onChange={(e) => {
                    const checked = e.target.checked;
                    setChangeAllTables(checked);
                    if (selectedCollation) {
                      handleSetCollation(selectedCollation, checked);
                    }
                  }}
                  className="accent-[var(--primary)]"
                />
                <span className="group-hover:text-[var(--foreground)] transition-colors">
                  Change all tables
                </span>
              </label>

              {executeMutation.isPending && (
                <div className="text-[9px] text-[var(--primary)] font-bold animate-pulse uppercase tracking-widest text-center">
                  Applying changes...
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Right Column: Maintenance & Insights */}
        <div className="space-y-6 lg:col-span-1">
          {/* Storage Insights */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <BarChart2 size={16} className="text-indigo-500" /> Storage
                Insights
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center justify-between text-xs mb-1">
                <span className="text-[var(--muted-foreground)]">
                  Total Schema Size
                </span>
                <span className="font-bold">{formatSize(totalSize)}</span>
              </div>
              <div className="space-y-3">
                {tableStorage.map((t, i) => (
                  <div key={i} className="space-y-1">
                    <div className="flex justify-between text-[10px]">
                      <span className="truncate max-w-[150px] font-medium">
                        {t.name}
                      </span>
                      <span className="text-[var(--muted-foreground)]">
                        {formatSize(t.size)}
                      </span>
                    </div>
                    <div className="h-1.5 w-full bg-[var(--muted)]/30 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-indigo-500 rounded-full"
                        style={{
                          width: `${Math.max(5, (t.size / (totalSize || 1)) * 100)}%`,
                        }}
                      />
                    </div>
                  </div>
                ))}
              </div>
              <div className="pt-2 text-center">
                <div className="text-[10px] text-[var(--muted-foreground)] mb-2 uppercase font-bold">
                  Quick Overview
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <div className="p-2 bg-[var(--muted)]/30 rounded-lg">
                    <div className="text-xs font-bold">
                      {tables?.length || 0}
                    </div>
                    <div className="text-[10px] text-[var(--muted-foreground)]">
                      Tables
                    </div>
                  </div>
                  <div className="p-2 bg-[var(--muted)]/30 rounded-lg">
                    <div className="text-xs font-bold">
                      {tables
                        ?.reduce((a, b) => a + b.row_count, 0)
                        .toLocaleString()}
                    </div>
                    <div className="text-[10px] text-[var(--muted-foreground)]">
                      Total Rows
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Maintenance */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <Wrench size={16} className="text-emerald-500" /> Maintenance
              </CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleMaintenance("CHECK")}
                disabled={maintenanceMutation.isPending}
                className="text-[10px] h-8"
              >
                Check All
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleMaintenance("ANALYZE")}
                disabled={maintenanceMutation.isPending}
                className="text-[10px] h-8"
              >
                Analyze All
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleMaintenance("OPTIMIZE")}
                disabled={maintenanceMutation.isPending}
                className="text-[10px] h-8"
              >
                Optimize All
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleMaintenance("REPAIR")}
                disabled={maintenanceMutation.isPending}
                className="text-[10px] h-8"
              >
                Repair All
              </Button>
            </CardContent>
            {maintenanceMutation.isPending && (
              <div className="px-6 pb-4">
                <div className="h-1 w-full bg-[var(--muted)]/30 overflow-hidden rounded-full">
                  <div
                    className="h-full bg-emerald-500 animate-pulse"
                    style={{ width: "100%" }}
                  />
                </div>
              </div>
            )}
          </Card>
        </div>
      </div>
    </div>
  );
}
