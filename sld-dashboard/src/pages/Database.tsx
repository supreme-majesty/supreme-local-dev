import { useState, useEffect, useRef, useMemo } from "react";
import {
  Database as DatabaseIcon,
  Table as TableIcon,
  Server,
  RefreshCw,
  Plus,
  Trash2,
  HardDrive,
  Camera,
  RotateCcw,
  ArrowRight,
  ArrowUp,
  ArrowDown,
  ChevronLeft,
  ChevronRight,
  Code2,
  Settings,
  Columns,
  Search,
  Zap,
  History as HistoryIcon,
  Download as DownloadIcon,
  FileDown,
  FileUp,
  Clock,
  Copy,
  X,
  ExternalLink,
} from "lucide-react";
import { formatBytes, formatDate } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
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
import { DatabaseStructure } from "@/components/database/DatabaseStructure";
import { TableCreator } from "@/components/database/TableCreator";
import { CloneDatabaseModal } from "@/components/database/CloneDatabaseModal";
import { CreateDatabaseModal } from "@/components/database/CreateDatabaseModal";
import { Modal } from "@/components/ui/Modal";
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
  useDeleteDatabaseMutation,
} from "@/hooks/use-database";
import { api } from "@/api/daemon";

export default function Database() {
  const [selectedDB, setSelectedDB] = useState<string | null>(null);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState("browse");
  const [dbDriver, setDbDriver] = useState<"mysql" | "postgres">(() => {
    const saved = localStorage.getItem("db_driver");
    return (saved === "postgres" ? "postgres" : "mysql") as
      | "mysql"
      | "postgres";
  });
  const [page, setPage] = useState(1);
  const [editingRow, setEditingRow] = useState<Record<string, any> | null>(
    null,
  );
  const [insertFormKey, setInsertFormKey] = useState(0);
  const [searchCriteria, setSearchCriteria] = useState<
    Record<string, { value: string; operator: string }>
  >({});
  const [searchResults, setSearchResults] = useState<any>(null);
  const [isSearching, setIsSearching] = useState(false);
  const [restoreAfterImport, setRestoreAfterImport] = useState(true);
  const [triggers, setTriggers] = useState<any[]>([]);
  const [loadingTriggers, setLoadingTriggers] = useState(false);
  const [selectedRows, setSelectedRows] = useState<Set<string>>(new Set());

  // Table controls state - load from localStorage
  const [perPage, setPerPage] = useState(() => {
    const saved = localStorage.getItem("db_perPage");
    return saved ? parseInt(saved, 10) : 25;
  });
  const [showAll, setShowAll] = useState(false);
  const [filterText, setFilterText] = useState("");
  const [sortCol, setSortCol] = useState("");
  const [sortOrder, setSortOrder] = useState<"ASC" | "DESC">("ASC");
  const [profiling, setProfiling] = useState(false);
  const [showExplainModal, setShowExplainModal] = useState(false);
  const [showPhpModal, setShowPhpModal] = useState(false);
  const [explainData, setExplainData] = useState<any>(null);

  // New Database Modal
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  // Clone Modal
  const [isCloneModalOpen, setIsCloneModalOpen] = useState(false);
  const [dbToClone, setDbToClone] = useState<string | null>(null);

  // Result modal state
  const [resultModal, setResultModal] = useState<{
    isOpen: boolean;
    title: string;
    content: string;
  }>({ isOpen: false, title: "", content: "" });

  // Inline cell editing state
  const [editingCell, setEditingCell] = useState<{
    rowIndex: number;
    colName: string;
    value: string;
    originalRow: Record<string, any>;
  } | null>(null);
  const [fkOptions, setFkOptions] = useState<
    { value: string; label: string }[] | null
  >(null);

  // Persist perPage to localStorage
  useEffect(() => {
    localStorage.setItem("db_perPage", String(perPage));
  }, [perPage]);

  const fileInputRef = useRef<HTMLInputElement>(null);

  // Queries - use table control options
  const {
    data: tableData,
    isLoading: loadingData,
    refetch: refetchTableData,
  } = useTableData(selectedDB, selectedTable, page, {
    perPage: showAll ? 10000 : perPage,
    sortCol,
    sortOrder,
    profile: profiling,
  });

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
  const deleteDatabaseMutation = useDeleteDatabaseMutation();

  const handleImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file && selectedDB) {
      importDatabaseMutation.mutate({
        file,
        database: selectedDB,
        restore: restoreAfterImport,
      });
      // Reset input
      e.target.value = "";
    }
  };

  const handleTruncateTable = () => {
    if (!selectedDB || !selectedTable) return;
    if (
      !confirm(
        `Are you sure you want to TRUNCATE table "${selectedTable}"? This will delete ALL data!`,
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
        `Are you sure you want to DROP table "${selectedTable}"? This action is IRREVERSIBLE!`,
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
      },
    );
  };

  const handleBulkDropTables = (tables: string[]) => {
    if (!selectedDB || tables.length === 0) return;
    if (
      !confirm(
        `Are you sure you want to DROP ${tables.length} tables? This is IRREVERSIBLE!`,
      )
    )
      return;

    const query = `DROP TABLE ${tables.map((t) => `\`${t}\``).join(", ")}`;

    executeQueryMutation.mutate({ database: selectedDB, query });
  };

  const handleBulkEmptyTables = async (tables: string[]) => {
    if (!selectedDB || tables.length === 0) return;
    if (
      !confirm(
        `Are you sure you want to TRUNCATE ${tables.length} tables? This will delete ALL data in them!`,
      )
    )
      return;

    for (const t of tables) {
      try {
        await executeQueryMutation.mutateAsync({
          database: selectedDB,
          query: `TRUNCATE TABLE \`${t}\``,
        });
      } catch (e) {
        console.error(`Failed to truncate ${t}`, e);
      }
    }
  };

  const handleBulkTableAction = async (tables: string[], action: string) => {
    if (!selectedDB || tables.length === 0) return;

    const tableList = tables.map((t) => `\`${t}\``).join(", ");
    let query = "";

    switch (action) {
      case "analyze":
        query = `ANALYZE TABLE ${tableList}`;
        break;
      case "check":
        query = `CHECK TABLE ${tableList}`;
        break;
      case "checksum":
        query = `CHECKSUM TABLE ${tableList}`;
        break;
      case "optimize":
        query = `OPTIMIZE TABLE ${tableList}`;
        break;
      case "repair":
        query = `REPAIR TABLE ${tableList}`;
        break;
      case "show_create": {
        // Show CREATE statements for all tables in a modal
        const results: string[] = [];
        for (const t of tables) {
          try {
            const result = await executeQueryMutation.mutateAsync({
              database: selectedDB,
              query: `SHOW CREATE TABLE \`${t}\``,
            });
            const createStmt = result.rows?.[0]?.["Create Table"] || "N/A";
            results.push(`-- Table: ${t}\n${createStmt};\n`);
          } catch (e) {
            results.push(
              `-- Table: ${t}\n-- Error: Failed to get CREATE statement\n`,
            );
          }
        }
        setResultModal({
          isOpen: true,
          title: `CREATE TABLE Statements (${tables.length} tables)`,
          content: results.join("\n"),
        });
        return;
      }
      case "export": {
        // Export selected tables as SQL
        const results: string[] = [];
        for (const t of tables) {
          try {
            const createResult = await executeQueryMutation.mutateAsync({
              database: selectedDB,
              query: `SHOW CREATE TABLE \`${t}\``,
            });
            results.push(`-- Table: ${t}`);
            results.push(`DROP TABLE IF EXISTS \`${t}\`;`);
            results.push(createResult.rows?.[0]?.["Create Table"] + ";");
            results.push("");
          } catch (e) {
            results.push(`-- Error exporting ${t}`);
          }
        }
        setResultModal({
          isOpen: true,
          title: `Export (${tables.length} tables)`,
          content: results.join("\n"),
        });
        return;
      }
      case "add_prefix": {
        const prefix = prompt("Enter prefix to add to table names:");
        if (!prefix) return;
        for (const t of tables) {
          try {
            await executeQueryMutation.mutateAsync({
              database: selectedDB,
              query: `RENAME TABLE \`${t}\` TO \`${prefix}${t}\``,
            });
          } catch (e) {
            console.error(`Failed to rename ${t}`, e);
          }
        }
        return;
      }
      case "replace_prefix": {
        const oldPrefix = prompt("Enter current prefix to replace:");
        if (!oldPrefix) return;
        const newPrefix = prompt("Enter new prefix:");
        if (newPrefix === null) return;
        for (const t of tables) {
          if (t.startsWith(oldPrefix)) {
            const newName = newPrefix + t.slice(oldPrefix.length);
            try {
              await executeQueryMutation.mutateAsync({
                database: selectedDB,
                query: `RENAME TABLE \`${t}\` TO \`${newName}\``,
              });
            } catch (e) {
              console.error(`Failed to rename ${t}`, e);
            }
          }
        }
        return;
      }
      case "copy_with_prefix": {
        const prefix = prompt("Enter prefix for copied tables:");
        if (!prefix) return;
        for (const t of tables) {
          try {
            await executeQueryMutation.mutateAsync({
              database: selectedDB,
              query: `CREATE TABLE \`${prefix}${t}\` LIKE \`${t}\``,
            });
            await executeQueryMutation.mutateAsync({
              database: selectedDB,
              query: `INSERT INTO \`${prefix}${t}\` SELECT * FROM \`${t}\``,
            });
          } catch (e) {
            console.error(`Failed to copy ${t}`, e);
          }
        }
        return;
      }
      case "copy": {
        // Copy table structure only
        for (const t of tables) {
          const newName = prompt(`Enter new name for copy of "${t}":`);
          if (!newName) continue;
          try {
            await executeQueryMutation.mutateAsync({
              database: selectedDB,
              query: `CREATE TABLE \`${newName}\` LIKE \`${t}\``,
            });
          } catch (e) {
            console.error(`Failed to copy ${t}`, e);
          }
        }
        return;
      }
      default:
        alert(`Action "${action}" is not yet implemented.`);
        return;
    }

    try {
      const result = await executeQueryMutation.mutateAsync({
        database: selectedDB,
        query,
      });
      // Format result for modal
      const rows = result.rows || [];
      const content =
        rows.length > 0
          ? rows
              .map((r: any) =>
                Object.entries(r)
                  .map(([k, v]) => `${k}: ${v}`)
                  .join("\n"),
              )
              .join("\n\n")
          : `Rows affected: ${result.rowCount || 0}`;
      setResultModal({
        isOpen: true,
        title: `${action.toUpperCase()} Result`,
        content,
      });
    } catch (e) {
      console.error(`Failed to ${action}`, e);
    }
  };

  const handleDropDatabase = () => {
    if (!selectedDB) return;
    if (
      !confirm(
        `Are you sure you want to DROP database "${selectedDB}"? This action is IRREVERSIBLE!`,
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
      },
    );
  };

  const handleDeleteDatabase = (db: string) => {
    if (
      !confirm(
        `Are you sure you want to DELETE database "${db}"? This action is IRREVERSIBLE!`,
      )
    )
      return;

    deleteDatabaseMutation.mutate(db, {
      onSuccess: () => {
        if (selectedDB === db) {
          setSelectedDB(null);
          setSelectedTable(null);
        }
      },
    });
  };

  const handleDeleteRow = (row: Record<string, any>) => {
    if (!selectedDB || !selectedTable || !pkCol) return;

    if (!confirm("Are you sure you want to delete this row?")) return;

    const query = `DELETE FROM \`${selectedTable}\` WHERE \`${pkCol}\` = ${escapeValue(
      row[pkCol],
    )} LIMIT 1`;
    executeQueryMutation.mutate({ database: selectedDB, query });
  };

  const handleSaveRow = (
    data: Record<string, any>,
    mode: "save" | "save_and_add" = "save",
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
        editingRow[pkCol],
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
      },
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
      " AND ",
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
      },
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
    setSelectedRows(new Set());
    if (!selectedTable && selectedDB) {
      setActiveTab("structure"); // DB view default
    } else if (selectedTable) {
      setActiveTab("browse"); // Table view default
    }
  }, [selectedDB, selectedTable]);

  // Clear selection on filter/page change
  useEffect(() => {
    setSelectedRows(new Set());
  }, [page, filterText, sortCol, sortOrder, showAll, searchResults]);

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
      },
    );
  };

  const handleCloneDatabase = (db: string) => {
    setDbToClone(db);
    setIsCloneModalOpen(true);
  };

  // Load triggers for the current database/table
  const loadTriggers = async () => {
    if (!selectedDB) return;

    setLoadingTriggers(true);
    try {
      // If table is selected, filter by table, otherwise show all for DB
      const query = selectedTable
        ? `SHOW TRIGGERS FROM \`${selectedDB}\` WHERE \`Table\` = '${selectedTable}'`
        : `SHOW TRIGGERS FROM \`${selectedDB}\``;

      const result = await executeQueryMutation.mutateAsync({
        database: selectedDB,
        query,
      });

      setTriggers(result.rows || []);
    } catch (err) {
      console.error("Failed to load triggers:", err);
      setTriggers([]);
    } finally {
      setLoadingTriggers(false);
    }
  };

  // Drop a trigger
  const handleDropTrigger = async (triggerName: string) => {
    if (!selectedDB) return;
    if (
      !confirm(
        `Are you sure you want to DROP trigger "${triggerName}"? This action is IRREVERSIBLE!`,
      )
    )
      return;

    executeQueryMutation.mutate(
      {
        database: selectedDB,
        query: `DROP TRIGGER \`${triggerName}\``,
      },
      {
        onSuccess: () => {
          // Reload triggers list
          loadTriggers();
        },
      },
    );
  };

  // Auto-load triggers when triggers tab is active
  useEffect(() => {
    if (activeTab === "triggers" && selectedDB) {
      loadTriggers();
    }
  }, [activeTab, selectedDB, selectedTable]);

  const currentSnapshotList =
    snapshots?.filter((s) => s.database === selectedDB) || [];

  // Filter rows client-side based on filterText
  const filteredRows = useMemo(() => {
    const rows = searchResults?.rows || tableData?.rows || [];
    if (!filterText.trim()) return rows;

    const searchLower = filterText.toLowerCase();
    return rows.filter((row: Record<string, any>) =>
      Object.values(row).some(
        (val) =>
          val !== null && String(val).toLowerCase().includes(searchLower),
      ),
    );
  }, [searchResults, tableData, filterText]);

  // Build current query for display
  const currentQuery = useMemo(() => {
    if (!selectedDB || !selectedTable) return "";
    let q = `SELECT * FROM \`${selectedTable}\``;
    if (sortCol) q += ` ORDER BY \`${sortCol}\` ${sortOrder}`;
    q += ` LIMIT ${showAll ? "ALL" : perPage}`;
    return q;
  }, [selectedDB, selectedTable, sortCol, sortOrder, perPage, showAll]);

  // Handle Explain SQL
  const handleExplainSQL = async () => {
    if (!selectedDB || !selectedTable) return;
    const query = `EXPLAIN SELECT * FROM \`${selectedTable}\`${
      sortCol ? ` ORDER BY \`${sortCol}\` ${sortOrder}` : ""
    }`;
    const result = await executeQueryMutation.mutateAsync({
      database: selectedDB,
      query,
    });
    setExplainData(result);
    setShowExplainModal(true);
  };

  // Generate PHP code
  const generatePhpCode = () => {
    if (!selectedDB || !selectedTable) return "";
    const query = `SELECT * FROM \`${selectedTable}\`${
      sortCol ? ` ORDER BY \`${sortCol}\` ${sortOrder}` : ""
    } LIMIT ${showAll ? 10000 : perPage}`;
    return `<?php
// Database connection
$mysqli = new mysqli("localhost", "root", "", "${selectedDB}");
if ($mysqli->connect_error) {
    die("Connection failed: " . $mysqli->connect_error);
}

// Execute query
$result = $mysqli->query("${query}");

// Fetch results
while ($row = $result->fetch_assoc()) {
    print_r($row);
}

$mysqli->close();
?>`;
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  // Toggle column sort - click same column toggles direction, new column sorts ASC
  const handleColumnSort = (colName: string) => {
    if (sortCol === colName) {
      setSortOrder(sortOrder === "ASC" ? "DESC" : "ASC");
    } else {
      setSortCol(colName);
      setSortOrder("ASC");
    }
    setPage(1);
  };

  // Inline editing functions
  const startInlineEdit = async (
    rowIndex: number,
    colName: string,
    value: any,
    row: Record<string, any>,
  ) => {
    setEditingCell({
      rowIndex,
      colName,
      value: value === null ? "" : String(value),
      originalRow: row,
    });
    setFkOptions(null);

    // Fetch FK options if applicable
    const cols = searchResults ? searchResults.columns : tableData?.columns;
    const colInfo = cols?.find(
      (c: any) => (typeof c === "string" ? c : c.name) === colName,
    );

    if (
      colInfo &&
      typeof colInfo !== "string" &&
      colInfo.foreign_key &&
      selectedDB
    ) {
      try {
        const opts = await api.getForeignValues(
          selectedDB,
          colInfo.foreign_key.table,
          colInfo.foreign_key.column,
        );
        setFkOptions(opts);
      } catch (err) {
        console.error("Failed to fetch FK options", err);
      }
    }
  };

  const cancelInlineEdit = () => {
    setEditingCell(null);
    setFkOptions(null);
  };

  const saveInlineEdit = async () => {
    if (!editingCell || !selectedDB || !selectedTable || !pkCol) return;

    const pkValue = getValue(editingCell.originalRow, pkCol);
    const newValue =
      editingCell.value === ""
        ? "NULL"
        : `'${editingCell.value.replace(/'/g, "\\'")}'`;

    const query = `UPDATE \`${selectedTable}\` SET \`${editingCell.colName}\` = ${newValue} WHERE \`${pkCol}\` = '${pkValue}'`;

    try {
      await executeQueryMutation.mutateAsync({
        database: selectedDB,
        query,
      });
      setEditingCell(null);
      refetchTableData();
    } catch (err) {
      console.error("Failed to save:", err);
    }
  };

  const handleInlineKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      saveInlineEdit();
    } else if (e.key === "Escape") {
      cancelInlineEdit();
    }
  };

  // Bulk Selection Logic
  const allSelected = useMemo(() => {
    if (!pkCol || filteredRows.length === 0) return false;
    return filteredRows.every((row: any) => {
      const val = getValue(row, pkCol);
      return val !== undefined && selectedRows.has(String(val));
    });
  }, [filteredRows, selectedRows, pkCol]);

  const handleSelectAll = (checked: boolean) => {
    if (!pkCol) return;
    const newSelected = new Set(selectedRows);
    filteredRows.forEach((row: any) => {
      const val = getValue(row, pkCol);
      if (val !== undefined) {
        const strVal = String(val);
        if (checked) newSelected.add(strVal);
        else newSelected.delete(strVal);
      }
    });
    setSelectedRows(newSelected);
  };

  const handleSelectRow = (pk: string, checked: boolean) => {
    const newSelected = new Set(selectedRows);
    if (checked) newSelected.add(pk);
    else newSelected.delete(pk);
    setSelectedRows(newSelected);
  };

  const handleBulkDelete = () => {
    if (!selectedDB || !selectedTable || selectedRows.size === 0 || !pkCol)
      return;
    if (!confirm(`Are you sure you want to delete ${selectedRows.size} rows?`))
      return;

    const ids = Array.from(selectedRows)
      .map((id) => `'${id.replace(/'/g, "\\'")}'`)
      .join(", ");
    const query = `DELETE FROM \`${selectedTable}\` WHERE \`${pkCol}\` IN (${ids})`;

    executeQueryMutation.mutate(
      { database: selectedDB, query },
      {
        onSuccess: () => {
          setSelectedRows(new Set());
          refetchTableData();
        },
      },
    );
  };

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
            handleSelectDb(db);
            setActiveTab("create_table");
          }}
          onCloneDatabase={handleCloneDatabase}
          onCreateDatabase={() => setIsCreateModalOpen(true)}
          onDeleteDatabase={handleDeleteDatabase}
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

          <div className="flex gap-2 items-center">
            {/* Driver Selector */}
            <select
              value={dbDriver}
              onChange={(e) => {
                const val = e.target.value as "mysql" | "postgres";
                setDbDriver(val);
                localStorage.setItem("db_driver", val);
                // Reset selection when driver changes
                setSelectedDB(null);
                setSelectedTable(null);
              }}
              className="bg-[var(--card)] border border-[var(--border)] text-[var(--foreground)] text-xs rounded-md px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--primary)]"
            >
              <option value="mysql">MySQL</option>
              <option value="postgres">PostgreSQL</option>
            </select>
          </div>
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
              {/* Action Links Bar */}
              <div className="flex items-center gap-4 text-sm text-[var(--muted-foreground)] border-b border-[var(--border)] pb-3">
                <button
                  className="hover:text-[var(--primary)] hover:underline"
                  onClick={() => refetchTableData()}
                >
                  Refresh
                </button>
                <button
                  className="hover:text-[var(--primary)] hover:underline"
                  onClick={handleExplainSQL}
                >
                  Explain SQL
                </button>
                <button
                  className="hover:text-[var(--primary)] hover:underline"
                  onClick={() => setShowPhpModal(true)}
                >
                  Create PHP code
                </button>
              </div>

              {/* Controls Bar */}
              <div className="flex flex-wrap items-center gap-4 py-2 px-3 bg-[var(--muted)]/30 rounded-md border border-[var(--border)]">
                {/* Profiling */}
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <Checkbox
                    checked={profiling}
                    onChange={(e) => setProfiling(e.target.checked)}
                  />
                  <span>Profiling</span>
                  {profiling && tableData?.query_time !== undefined && (
                    <span className="text-[var(--primary)] font-medium">
                      ({tableData.query_time.toFixed(4)}s)
                    </span>
                  )}
                </label>

                {selectedRows.size > 0 && pkCol && (
                  <>
                    <div className="h-4 w-px bg-[var(--border)]" />
                    <div className="flex items-center gap-2 text-sm">
                      <span className="font-medium text-[var(--primary)]">
                        {selectedRows.size} selected
                      </span>
                      <select
                        className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-sm text-[var(--foreground)]"
                        value=""
                        onChange={(e) => {
                          const val = e.target.value;
                          if (!val) return;

                          if (val === "delete") {
                            handleBulkDelete();
                          } else if (val === "export") {
                            // Export selected rows as SQL INSERT statements
                            const selectedData = filteredRows.filter(
                              (row: any) => {
                                const pk = getValue(row, pkCol);
                                return (
                                  pk !== undefined &&
                                  selectedRows.has(String(pk))
                                );
                              },
                            );
                            const cols =
                              (searchResults || tableData)?.columns || [];
                            const colNames = cols.map((c: any) =>
                              typeof c === "string" ? c : c.name,
                            );

                            const inserts = selectedData.map((row: any) => {
                              const values = colNames.map((col: string) => {
                                const val = getValue(row, col);
                                if (val === null) return "NULL";
                                if (typeof val === "number") return val;
                                return `'${String(val).replace(/'/g, "\\'")}'`;
                              });
                              return `INSERT INTO \`${selectedTable}\` (\`${colNames.join(
                                "`, `",
                              )}\`) VALUES (${values.join(", ")});`;
                            });

                            setResultModal({
                              isOpen: true,
                              title: `Export ${selectedData.length} rows`,
                              content: inserts.join("\n"),
                            });
                            setSelectedRows(new Set());
                          } else if (val === "copy") {
                            // Copy selected rows to clipboard as TSV
                            const selectedData = filteredRows.filter(
                              (row: any) => {
                                const pk = getValue(row, pkCol);
                                return (
                                  pk !== undefined &&
                                  selectedRows.has(String(pk))
                                );
                              },
                            );
                            const cols =
                              (searchResults || tableData)?.columns || [];
                            const colNames = cols.map((c: any) =>
                              typeof c === "string" ? c : c.name,
                            );

                            const header = colNames.join("\t");
                            const rows = selectedData.map((row: any) =>
                              colNames
                                .map((col: string) =>
                                  String(getValue(row, col) ?? ""),
                                )
                                .join("\t"),
                            );
                            const tsv = [header, ...rows].join("\n");

                            navigator.clipboard.writeText(tsv);
                            alert(
                              `Copied ${selectedData.length} rows to clipboard!`,
                            );
                            setSelectedRows(new Set());
                          }
                          e.target.value = "";
                        }}
                      >
                        <option value="">With selected:</option>
                        <option value="edit">Edit</option>
                        <option value="copy">Copy</option>
                        <option value="delete">Delete</option>
                        <option value="export">Export</option>
                      </select>
                    </div>
                  </>
                )}

                <div className="h-4 w-px bg-[var(--border)]" />

                {/* Show All */}
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <Checkbox
                    checked={showAll}
                    onChange={(e) => {
                      setShowAll(e.target.checked);
                      setPage(1);
                    }}
                  />
                  <span>Show all</span>
                </label>

                <div className="h-4 w-px bg-[var(--border)]" />

                {/* Number of rows */}
                <div className="flex items-center gap-2 text-sm">
                  <span>Number of rows:</span>
                  <select
                    value={perPage}
                    onChange={(e) => {
                      setPerPage(Number(e.target.value));
                      setPage(1);
                    }}
                    disabled={showAll}
                    className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-sm disabled:opacity-50"
                  >
                    <option value={25}>25</option>
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                    <option value={250}>250</option>
                    <option value={500}>500</option>
                  </select>
                </div>

                <div className="h-4 w-px bg-[var(--border)]" />

                {/* Filter rows */}
                <div className="flex items-center gap-2 text-sm">
                  <span>Filter rows:</span>
                  <input
                    type="text"
                    value={filterText}
                    onChange={(e) => setFilterText(e.target.value)}
                    placeholder="Search this table"
                    className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-sm w-40"
                  />
                  {filterText && (
                    <button
                      onClick={() => setFilterText("")}
                      className="text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                    >
                      <X size={14} />
                    </button>
                  )}
                </div>

                <div className="h-4 w-px bg-[var(--border)]" />

                {/* Sort by key */}
                <div className="flex items-center gap-2 text-sm">
                  <span>Sort by key:</span>
                  <select
                    value={sortCol}
                    onChange={(e) => {
                      setSortCol(e.target.value);
                      setPage(1);
                    }}
                    className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-sm"
                  >
                    <option value="">None</option>
                    {tableSchema?.map((col) => (
                      <option key={col.name} value={col.name}>
                        {col.name}
                      </option>
                    ))}
                  </select>
                  {sortCol && (
                    <button
                      onClick={() =>
                        setSortOrder(sortOrder === "ASC" ? "DESC" : "ASC")
                      }
                      className="px-2 py-1 text-xs bg-[var(--background)] border border-[var(--border)] rounded hover:bg-[var(--muted)]"
                    >
                      {sortOrder}
                    </button>
                  )}
                </div>
              </div>

              {/* Query Info */}
              {profiling && tableData?.query_time !== undefined && (
                <div className="text-sm text-[var(--muted-foreground)] bg-[#ffffcc] text-black px-3 py-2 rounded border border-yellow-400">
                  <Clock size={14} className="inline mr-2" />
                  Showing rows 0 - {filteredRows.length - 1} (
                  {tableData?.total || 0} total, Query took{" "}
                  {tableData.query_time.toFixed(4)} seconds.)
                </div>
              )}

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
                      <thead className="text-xs text-[var(--muted-foreground)] bg-[var(--muted)]/50 border-b border-[var(--border)]">
                        <tr>
                          {pkCol && (
                            <th className="w-8 px-4 py-3 text-center">
                              <Checkbox
                                checked={allSelected}
                                onChange={(e) =>
                                  handleSelectAll(e.target.checked)
                                }
                              />
                            </th>
                          )}
                          <th className="px-4 py-3 font-medium">Actions</th>
                          {(searchResults || tableData).columns?.map(
                            (col: any) => {
                              const colName =
                                typeof col === "string" ? col : col.name;
                              const colType =
                                typeof col === "string" ? "" : col.type;
                              const isSorted = sortCol === colName;
                              return (
                                <th
                                  key={colName}
                                  className="px-4 py-3 font-medium whitespace-nowrap cursor-pointer hover:bg-[var(--muted)]/50 select-none"
                                  onClick={() => handleColumnSort(colName)}
                                >
                                  <div className="flex items-center gap-1">
                                    {colName}
                                    <span className="text-[10px] text-[var(--muted-foreground)] normal-case">
                                      {colType}
                                    </span>
                                    {isSorted &&
                                      (sortOrder === "ASC" ? (
                                        <ArrowUp
                                          size={12}
                                          className="text-[var(--primary)]"
                                        />
                                      ) : (
                                        <ArrowDown
                                          size={12}
                                          className="text-[var(--primary)]"
                                        />
                                      ))}
                                  </div>
                                </th>
                              );
                            },
                          )}
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-[var(--border)]">
                        {filteredRows.map((row: any, i: number) => (
                          <tr
                            key={i}
                            className="hover:bg-[var(--muted)]/30 group"
                          >
                            {pkCol && (
                              <td className="px-4 py-2 w-8 text-center">
                                <Checkbox
                                  checked={(() => {
                                    const val = getValue(row, pkCol);
                                    return (
                                      val !== undefined &&
                                      selectedRows.has(String(val))
                                    );
                                  })()}
                                  onChange={(e) => {
                                    const val = getValue(row, pkCol);
                                    if (val !== undefined)
                                      handleSelectRow(
                                        String(val),
                                        e.target.checked,
                                      );
                                  }}
                                />
                              </td>
                            )}
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
                                const val = getValue(row, colName);
                                const isEditing =
                                  editingCell?.rowIndex === i &&
                                  editingCell?.colName === colName;

                                const isFK =
                                  typeof col !== "string" && !!col.foreign_key;

                                return (
                                  <td
                                    key={colName}
                                    className={`px-4 py-2 whitespace-nowrap max-w-[300px] ${
                                      isEditing
                                        ? ""
                                        : `truncate cursor-pointer hover:bg-[var(--muted)]/30 ${
                                            isFK
                                              ? "text-[var(--primary)] hover:underline"
                                              : ""
                                          }`
                                    }`}
                                    onDoubleClick={() => {
                                      if (!isEditing && pkCol) {
                                        startInlineEdit(i, colName, val, row);
                                      }
                                    }}
                                    onClick={undefined} // Remove cell click navigation
                                    title={
                                      pkCol
                                        ? "Double-click to edit"
                                        : "No primary key - cannot edit"
                                    }
                                  >
                                    {isEditing ? (
                                      <div className="flex items-center gap-1 w-full min-w-[200px]">
                                        {(() => {
                                          const type =
                                            typeof col === "string"
                                              ? ""
                                              : col.type?.toLowerCase() || "";
                                          const isDate =
                                            type.includes("datetime") ||
                                            type.includes("timestamp");

                                          // FK Dropdown
                                          if (isFK && fkOptions) {
                                            return (
                                              <select
                                                value={editingCell.value}
                                                onChange={(e) =>
                                                  setEditingCell({
                                                    ...editingCell,
                                                    value: e.target.value,
                                                  })
                                                }
                                                onKeyDown={handleInlineKeyDown}
                                                onBlur={saveInlineEdit}
                                                autoFocus
                                                className="w-full bg-[var(--background)] border border-[var(--primary)] rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-[var(--primary)]"
                                              >
                                                <option value="">
                                                  Select...
                                                </option>
                                                {fkOptions.map((opt) => (
                                                  <option
                                                    key={opt.value}
                                                    value={opt.value}
                                                  >
                                                    {opt.label}
                                                  </option>
                                                ))}
                                              </select>
                                            );
                                          }

                                          // Format for datetime-local: YYYY-MM-DDThh:mm:ss
                                          const displayValue =
                                            isDate && editingCell.value
                                              ? editingCell.value.replace(
                                                  " ",
                                                  "T",
                                                )
                                              : editingCell.value;

                                          return (
                                            <>
                                              <input
                                                type={
                                                  isDate
                                                    ? "datetime-local"
                                                    : "text"
                                                }
                                                value={displayValue}
                                                onChange={(e) => {
                                                  let newVal = e.target.value;
                                                  if (isDate) {
                                                    // Convert back to MySQL format: YYYY-MM-DD hh:mm:ss
                                                    newVal = newVal.replace(
                                                      "T",
                                                      " ",
                                                    );
                                                  }
                                                  setEditingCell({
                                                    ...editingCell,
                                                    value: newVal,
                                                  });
                                                }}
                                                onKeyDown={(e) => {
                                                  handleInlineKeyDown(e);
                                                }}
                                                // Remove onBlur for dates because clicking the picker/Now button triggers blur
                                                onBlur={
                                                  isDate
                                                    ? undefined
                                                    : saveInlineEdit
                                                }
                                                autoFocus
                                                step="1"
                                                className="w-full bg-[var(--background)] border border-[var(--primary)] rounded px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-[var(--primary)]"
                                              />
                                              {isDate && (
                                                <>
                                                  <button
                                                    onClick={(e) => {
                                                      e.stopPropagation(); // Prevent row click
                                                      const now = new Date();
                                                      // Construct local YYYY-MM-DD hh:mm:ss
                                                      const localIso = new Date(
                                                        now.getTime() -
                                                          now.getTimezoneOffset() *
                                                            60000,
                                                      )
                                                        .toISOString()
                                                        .slice(0, 19)
                                                        .replace("T", " ");

                                                      setEditingCell({
                                                        ...editingCell,
                                                        value: localIso,
                                                      });
                                                    }}
                                                    className="p-1 text-[var(--muted-foreground)] hover:text-[var(--primary)] bg-[var(--muted)] rounded border border-[var(--border)]"
                                                    title="Set to NOW"
                                                  >
                                                    <Clock size={14} />
                                                  </button>
                                                  <button
                                                    onClick={saveInlineEdit}
                                                    className="p-1 hover:text-green-500 bg-[var(--muted)] rounded border border-[var(--border)]"
                                                    title="Save"
                                                  >
                                                    <ArrowRight size={14} />
                                                  </button>
                                                </>
                                              )}
                                            </>
                                          );
                                        })()}
                                      </div>
                                    ) : (
                                      <div className="flex items-center justify-between group/cell relative min-h-[20px]">
                                        <span
                                          className={
                                            val === null
                                              ? "text-[var(--muted-foreground)] italic"
                                              : ""
                                          }
                                        >
                                          {val === null ? (
                                            "NULL"
                                          ) : val === undefined ? (
                                            <span className="text-red-400 text-[10px]">
                                              (missing)
                                            </span>
                                          ) : (
                                            String(val)
                                          )}
                                        </span>
                                        {!isEditing && isFK && val && (
                                          <button
                                            className="opacity-0 group-hover/cell:opacity-100 p-0.5 text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-opacity bg-[var(--card)] rounded shadow-sm border border-[var(--border)] absolute right-0 top-1/2 -translate-y-1/2 z-10 cursor-pointer"
                                            onClick={(e) => {
                                              e.preventDefault();
                                              e.stopPropagation();

                                              const fkTable =
                                                col.foreign_key!.table;
                                              const fkColumn =
                                                col.foreign_key!.column;
                                              const fkValue = String(val);

                                              setSelectedDB(selectedDB!);
                                              setSelectedTable(fkTable);
                                              setPage(1);
                                              setSortCol("");
                                              setFilterText("");

                                              // Set search criteria to filter by FK value
                                              setSearchCriteria({
                                                [fkColumn]: {
                                                  value: fkValue,
                                                  operator: "=",
                                                },
                                              });

                                              // Trigger search after navigating
                                              setTimeout(() => {
                                                setActiveTab("search");
                                              }, 100);
                                            }}
                                            title={`Navigate to ${
                                              col.foreign_key!.table
                                            } where ${
                                              col.foreign_key!.column
                                            } = ${val}`}
                                          >
                                            <ExternalLink size={10} />
                                          </button>
                                        )}
                                      </div>
                                    )}
                                  </td>
                                );
                              },
                            )}
                          </tr>
                        ))}
                        {filteredRows.length === 0 && (
                          <tr>
                            <td
                              colSpan={
                                ((searchResults || tableData).columns || [])
                                  .length + 1
                              }
                              className="px-4 py-8 text-center text-[var(--muted-foreground)] italic"
                            >
                              {filterText
                                ? "No matching rows"
                                : "No data found"}
                            </td>
                          </tr>
                        )}

                        {/* Pagination / Status */}
                        <tr>
                          <td
                            colSpan={
                              ((searchResults || tableData).columns || [])
                                .length + (pkCol ? 2 : 1)
                            }
                            className="px-4 py-2 text-xs text-[var(--muted-foreground)] border-t border-[var(--border)]"
                          >
                            <div className="flex justify-between items-center">
                              <span>
                                Showing {filteredRows.length} of{" "}
                                {tableData?.total || 0} rows
                                {filterText && (
                                  <span className="ml-1 text-blue-400">
                                    (filtered by "{filterText}")
                                  </span>
                                )}
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
                              <div className="flex items-center gap-2">
                                {!showAll &&
                                  !searchResults &&
                                  (tableData?.total_pages ?? 1) > 1 && (
                                    <>
                                      <Button
                                        variant="secondary"
                                        size="sm"
                                        disabled={page <= 1}
                                        onClick={() => setPage(1)}
                                        className="px-2"
                                      >
                                        First
                                      </Button>
                                      <Button
                                        variant="secondary"
                                        size="sm"
                                        disabled={page <= 1}
                                        onClick={() => setPage((p) => p - 1)}
                                        className="px-2"
                                      >
                                        <ChevronLeft size={14} />
                                      </Button>
                                      <span className="flex items-center gap-1 text-xs">
                                        Page
                                        <input
                                          type="number"
                                          value={page}
                                          onChange={(e) => {
                                            const val = parseInt(
                                              e.target.value,
                                              10,
                                            );
                                            if (
                                              val >= 1 &&
                                              val <=
                                                (tableData?.total_pages || 1)
                                            ) {
                                              setPage(val);
                                            }
                                          }}
                                          className="w-12 text-center bg-[var(--background)] border border-[var(--border)] rounded px-1 py-0.5"
                                          min={1}
                                          max={tableData?.total_pages || 1}
                                        />
                                        of {tableData?.total_pages || 1}
                                      </span>
                                      <Button
                                        variant="secondary"
                                        size="sm"
                                        disabled={
                                          page >= (tableData?.total_pages || 1)
                                        }
                                        onClick={() => setPage((p) => p + 1)}
                                        className="px-2"
                                      >
                                        <ChevronRight size={14} />
                                      </Button>
                                      <Button
                                        variant="secondary"
                                        size="sm"
                                        disabled={
                                          page >= (tableData?.total_pages || 1)
                                        }
                                        onClick={() =>
                                          setPage(tableData?.total_pages || 1)
                                        }
                                        className="px-2"
                                      >
                                        Last
                                      </Button>
                                    </>
                                  )}
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
                <thead className="text-xs text-[var(--muted-foreground)] bg-[var(--muted)]/50 border-b border-[var(--border)]">
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
                onClick={() =>
                  createSnapshotMutation.mutate({ database: selectedDB })
                }
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
                          "_blank",
                        )
                      }
                    >
                      <FileDown size={14} className="mr-2" /> Download{" "}
                      {snap.table || snap.database}
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

          {/* Context: DB, Tab: Tables (Structure view for DB) */}
          {activeTab === "structure" && selectedDB && !selectedTable && (
            <DatabaseStructure
              database={selectedDB}
              onSelectTable={(table) => {
                setSelectedTable(table);
                // Default to browse or structure? usually structure if clicking from list
                setActiveTab("browse");
              }}
              onDropTable={(table) => {
                if (
                  confirm(
                    `Are you sure you want to DROP table '${table}'? This cannot be undone.`,
                  )
                ) {
                  executeQueryMutation.mutate({
                    database: selectedDB,
                    query: `DROP TABLE \`${table}\``,
                  });
                }
              }}
              onEmptyTable={(table) => {
                if (
                  confirm(
                    `Are you sure you want to TRUNCATE table '${table}'? This will delete all rows.`,
                  )
                ) {
                  executeQueryMutation.mutate({
                    database: selectedDB,
                    query: `TRUNCATE TABLE \`${table}\``,
                  });
                }
              }}
              onBulkDrop={handleBulkDropTables}
              onBulkEmpty={handleBulkEmptyTables}
              onBulkAction={handleBulkTableAction}
            />
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
            <div className="w-full mt-8">
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
                  <div className="flex flex-col gap-6">
                    <div
                      className="border-2 border-dashed border-[var(--border)] rounded-lg p-10 text-center hover:bg-[var(--muted)]/20 transition-all cursor-pointer group"
                      onClick={() => fileInputRef.current?.click()}
                    >
                      <FileUp className="mx-auto h-12 w-12 text-[var(--muted-foreground)] group-hover:text-[var(--primary)] mb-4 transition-colors" />
                      <p className="font-medium mb-1">Select SQL File</p>
                      <p className="text-sm text-[var(--muted-foreground)] mb-6">
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
                        variant="secondary"
                        loading={importDatabaseMutation.isPending}
                      >
                        Choose File
                      </Button>
                    </div>

                    <div className="flex items-center gap-3 p-4 bg-[var(--muted)]/30 rounded-lg">
                      <Checkbox
                        id="restore-after-import"
                        checked={restoreAfterImport}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                          setRestoreAfterImport(e.target.checked)
                        }
                      />
                      <label
                        htmlFor="restore-after-import"
                        className="text-sm cursor-pointer select-none"
                      >
                        <span className="font-medium">
                          Restore after upload
                        </span>
                        <p className="text-xs text-[var(--muted-foreground)]">
                          Database contents will be replaced with the SQL file
                          data.
                        </p>
                      </label>
                    </div>

                    <div className="text-xs text-[var(--muted-foreground)] text-center italic">
                      Note: Large files might take a few moments to process.
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
            <div className="w-full flex flex-col gap-6">
              {/* Context: Table, Tab: Operations */}
              {activeTab === "operations" && selectedTable && (
                <div className="space-y-6 w-full">
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
                                  "rename-table-input",
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
                                  },
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
                                  "copy-table-input",
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
                                  },
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
                <Button onClick={() => setIsCreateModalOpen(true)}>
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

          {/* Context: Export */}
          {activeTab === "export" && selectedDB && (
            <div className="w-full mt-8">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
                <Card
                  className="hover:border-[var(--primary)] transition-colors cursor-pointer"
                  onClick={() =>
                    createSnapshotMutation.mutate({ database: selectedDB })
                  }
                >
                  <CardHeader>
                    <div className="w-10 h-10 rounded-full bg-[var(--primary)]/10 flex items-center justify-center text-[var(--primary)] mb-2">
                      <DatabaseIcon size={20} />
                    </div>
                    <CardTitle className="text-lg">Database Snapshot</CardTitle>
                    <CardDescription>
                      Export the entire <strong>{selectedDB}</strong> database
                      structure and data.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Button
                      variant="secondary"
                      className="w-full"
                      loading={
                        createSnapshotMutation.isPending && !selectedTable
                      }
                    >
                      Create Full Backup
                    </Button>
                  </CardContent>
                </Card>

                {selectedTable && (
                  <Card
                    className="hover:border-[var(--primary)] transition-colors cursor-pointer"
                    onClick={() =>
                      createSnapshotMutation.mutate({
                        database: selectedDB,
                        table: selectedTable,
                      })
                    }
                  >
                    <CardHeader>
                      <div className="w-10 h-10 rounded-full bg-[var(--secondary)]/10 flex items-center justify-center text-[var(--secondary)] mb-2">
                        <TableIcon size={20} />
                      </div>
                      <CardTitle className="text-lg">Table Export</CardTitle>
                      <CardDescription>
                        Export only the <strong>{selectedTable}</strong> table
                        from {selectedDB}.
                      </CardDescription>
                    </CardHeader>
                    <CardContent>
                      <Button
                        variant="secondary"
                        className="w-full"
                        loading={
                          createSnapshotMutation.isPending && !!selectedTable
                        }
                      >
                        Export Table
                      </Button>
                    </CardContent>
                  </Card>
                )}
              </div>

              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <HistoryIcon size={18} /> Recent Exports
                  </CardTitle>
                  <CardDescription>
                    Manage and download your database snapshots.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="border border-[var(--border)] rounded-md overflow-hidden">
                    <table className="w-full text-sm text-left">
                      <thead className="bg-[var(--muted)]/50 border-b border-[var(--border)] text-xs uppercase text-[var(--muted-foreground)]">
                        <tr>
                          <th className="px-4 py-3 font-medium">Filename</th>
                          <th className="px-4 py-3 font-medium">Size</th>
                          <th className="px-4 py-3 font-medium">Created</th>
                          <th className="px-4 py-3 font-medium text-right">
                            Actions
                          </th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-[var(--border)] bg-[var(--card)]">
                        {!snapshots ||
                        snapshots.filter((s) => s.database === selectedDB)
                          .length === 0 ? (
                          <tr>
                            <td
                              colSpan={4}
                              className="px-4 py-8 text-center text-[var(--muted-foreground)] italic"
                            >
                              No snapshots found for this database.
                            </td>
                          </tr>
                        ) : (
                          snapshots
                            .filter((s) => s.database === selectedDB)
                            .map((s) => (
                              <tr
                                key={s.id}
                                className="hover:bg-[var(--muted)]/10 transition-colors"
                              >
                                <td className="px-4 py-3 font-mono text-xs">
                                  {s.filename}
                                </td>
                                <td className="px-4 py-3 text-xs text-[var(--muted-foreground)]">
                                  {(s.size / 1024).toFixed(1)} KB
                                </td>
                                <td className="px-4 py-3 text-xs text-[var(--muted-foreground)]">
                                  {new Date(s.created_at).toLocaleString()}
                                </td>
                                <td className="px-4 py-3 text-right">
                                  <div className="flex justify-end gap-1">
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      className="h-8 w-8 p-0"
                                      title={`Download ${
                                        s.table
                                          ? `${s.database}.${s.table}`
                                          : s.database
                                      }`}
                                      onClick={() =>
                                        window.open(
                                          `/api/db/snapshots/download?id=${s.id}`,
                                        )
                                      }
                                    >
                                      <DownloadIcon size={14} />
                                    </Button>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      className="h-8 w-8 p-0 text-[var(--secondary)]"
                                      title="Restore"
                                      onClick={() => {
                                        if (
                                          confirm(
                                            "Restore this snapshot? Current database data will be overwritten!",
                                          )
                                        ) {
                                          restoreSnapshotMutation.mutate(s.id);
                                        }
                                      }}
                                    >
                                      <RotateCcw size={14} />
                                    </Button>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      className="h-8 w-8 p-0 text-red-500"
                                      title="Delete"
                                      onClick={() => {
                                        if (confirm("Delete this snapshot?")) {
                                          deleteSnapshotMutation.mutate(s.id);
                                        }
                                      }}
                                    >
                                      <Trash2 size={14} />
                                    </Button>
                                  </div>
                                </td>
                              </tr>
                            ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </CardContent>
              </Card>
            </div>
          )}

          {/* Context: Other Tabs Placeholders */}
          {activeTab === "triggers" && selectedTable && (
            <div className="space-y-4">
              <Card>
                <CardHeader className="flex flex-row items-center justify-between">
                  <div>
                    <CardTitle className="text-lg flex items-center gap-2">
                      <Zap size={18} /> Triggers
                    </CardTitle>
                    <CardDescription>
                      Triggers for table <strong>{selectedTable}</strong>
                    </CardDescription>
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={loadTriggers}
                    disabled={loadingTriggers}
                  >
                    <RefreshCw
                      size={14}
                      className={loadingTriggers ? "animate-spin" : ""}
                    />
                    Refresh
                  </Button>
                </CardHeader>
                <CardContent>
                  {loadingTriggers ? (
                    <div className="flex justify-center py-10">
                      <RefreshCw className="animate-spin text-[var(--muted-foreground)]" />
                    </div>
                  ) : triggers.length === 0 ? (
                    <div className="text-center py-12 text-[var(--muted-foreground)]">
                      No triggers found for this table
                    </div>
                  ) : (
                    <div className="border border-[var(--border)] rounded-md overflow-hidden">
                      <table className="w-full text-sm text-left">
                        <thead className="text-xs text-[var(--muted-foreground)] uppercase bg-[var(--muted)]/50 border-b border-[var(--border)]">
                          <tr>
                            <th className="px-4 py-3 font-medium">Name</th>
                            <th className="px-4 py-3 font-medium">Event</th>
                            <th className="px-4 py-3 font-medium">Timing</th>
                            <th className="px-4 py-3 font-medium">Table</th>
                            <th className="px-4 py-3 font-medium">Statement</th>
                            <th className="px-4 py-3 font-medium w-24">
                              Actions
                            </th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-[var(--border)]">
                          {triggers.map((trigger: any, idx: number) => (
                            <tr
                              key={idx}
                              className="hover:bg-[var(--muted)]/30"
                            >
                              <td className="px-4 py-3 font-medium">
                                {trigger.Trigger}
                              </td>
                              <td className="px-4 py-3">
                                <Badge variant="secondary">
                                  {trigger.Event}
                                </Badge>
                              </td>
                              <td className="px-4 py-3">
                                <Badge
                                  variant={
                                    trigger.Timing === "BEFORE"
                                      ? "secondary"
                                      : "outline"
                                  }
                                >
                                  {trigger.Timing}
                                </Badge>
                              </td>
                              <td className="px-4 py-3">{trigger.Table}</td>
                              <td className="px-4 py-3 max-w-[300px] truncate font-mono text-xs">
                                {trigger.Statement}
                              </td>
                              <td className="px-4 py-3">
                                <Button
                                  variant="danger"
                                  size="sm"
                                  onClick={() =>
                                    handleDropTrigger(trigger.Trigger)
                                  }
                                  disabled={executeQueryMutation.isPending}
                                >
                                  <Trash2 size={14} />
                                  Drop
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>
          )}
        </div>
      </div>

      {/* Explain SQL Modal */}
      {showExplainModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-[var(--card)] rounded-lg shadow-xl max-w-2xl w-full m-4 max-h-[80vh] overflow-hidden">
            <div className="flex items-center justify-between p-4 border-b border-[var(--border)]">
              <h3 className="font-semibold">Explain SQL</h3>
              <button
                onClick={() => setShowExplainModal(false)}
                className="text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
              >
                <X size={18} />
              </button>
            </div>
            <div className="p-4 overflow-auto max-h-[60vh]">
              <div className="mb-3 p-2 bg-[var(--muted)]/50 rounded font-mono text-sm">
                {currentQuery}
              </div>
              {explainData?.rows?.length > 0 ? (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-[var(--border)]">
                      {explainData.columns?.map((col: string) => (
                        <th key={col} className="px-2 py-1 text-left text-xs">
                          {col}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {explainData.rows.map((row: any, i: number) => (
                      <tr key={i} className="border-b border-[var(--border)]">
                        {explainData.columns?.map((col: string) => (
                          <td key={col} className="px-2 py-1 text-xs">
                            {row[col] ?? "-"}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="text-[var(--muted-foreground)]">
                  No EXPLAIN data available
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Create PHP Code Modal */}
      {showPhpModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-[var(--card)] rounded-lg shadow-xl max-w-2xl w-full m-4 max-h-[80vh] overflow-hidden">
            <div className="flex items-center justify-between p-4 border-b border-[var(--border)]">
              <h3 className="font-semibold">PHP Code</h3>
              <button
                onClick={() => setShowPhpModal(false)}
                className="text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
              >
                <X size={18} />
              </button>
            </div>
            <div className="p-4 overflow-auto max-h-[60vh]">
              <div className="flex justify-end mb-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => copyToClipboard(generatePhpCode())}
                  className="gap-1"
                >
                  <Copy size={14} /> Copy
                </Button>
              </div>
              <pre className="bg-[var(--muted)]/50 p-4 rounded text-sm font-mono overflow-x-auto whitespace-pre-wrap">
                {generatePhpCode()}
              </pre>
            </div>
          </div>
        </div>
      )}

      {/* Result Modal for bulk actions */}
      <Modal
        isOpen={resultModal.isOpen}
        onClose={() =>
          setResultModal({ isOpen: false, title: "", content: "" })
        }
        title={resultModal.title}
        footer={
          <div className="flex gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                navigator.clipboard.writeText(resultModal.content);
                alert("Copied to clipboard!");
              }}
              className="gap-1"
            >
              <Copy size={14} /> Copy to Clipboard
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                const element = document.createElement("a");
                const file = new Blob([resultModal.content], {
                  type: "text/plain",
                });
                element.href = URL.createObjectURL(file);
                element.download = `${resultModal.title.replace(
                  /\s+/g,
                  "_",
                )}.sql`;
                document.body.appendChild(element);
                element.click();
                document.body.removeChild(element);
              }}
              className="gap-1"
            >
              <DownloadIcon size={14} /> Download as SQL
            </Button>
          </div>
        }
      >
        <div className="max-h-[60vh] overflow-auto">
          <pre className="bg-[var(--muted)]/50 p-4 rounded text-sm font-mono overflow-x-auto whitespace-pre-wrap">
            {resultModal.content}
          </pre>
        </div>
      </Modal>
      {/* Clone Database Modal */}
      {dbToClone && (
        <CloneDatabaseModal
          isOpen={isCloneModalOpen}
          onClose={() => setIsCloneModalOpen(false)}
          sourceDatabase={dbToClone}
        />
      )}

      <CreateDatabaseModal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
      />
    </div>
  );
}
