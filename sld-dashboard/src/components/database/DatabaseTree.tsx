import { useState, useEffect, useMemo } from "react";
import {
  Database as DatabaseIcon,
  Table as TableIcon,
  ChevronRight,
  ChevronDown,
  Server,
  RefreshCw,
  Plus,
  Search,
  Folder,
  Copy,
  Trash2,
  Edit2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import {
  useDatabases,
  useDatabaseCatalog,
  useTables,
  useRenameDatabaseMutation,
} from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ContextMenu } from "@/components/ui/ContextMenu";
import {
  buildDatabaseTree,
  type TreeNode,
  type TreeDatabaseInput,
} from "./tree-utils";

interface DatabaseTreeProps {
  selectedDb: string | null;
  selectedTable: string | null;
  activeSourceId?: string;
  serverLabel?: string;
  connected?: boolean;
  onSelectDb: (db: string, sourceId?: string) => void;
  onSelectTable: (db: string, table: string, sourceId?: string) => void;
  onCreateTable: (db: string, sourceId?: string) => void;
  onCloneDatabase: (db: string, sourceId?: string) => void;
  onRenameDatabase?: (db: string, sourceId?: string) => void;
  onCreateDatabase: () => void;
  onDeleteDatabase: (db: string, sourceId?: string) => void;
}

function TableNode({
  tableName,
  isSelected,
  onSelect,
  isMainAction, // For "New"
}: {
  tableName: string;
  isSelected: boolean;
  onSelect: () => void;
  isMainAction?: boolean;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-6 py-1.5 text-sm cursor-pointer transition-colors",
        isSelected
          ? "bg-[var(--primary)]/10 text-[var(--primary)] border-r-2 border-[var(--primary)]"
          : "text-[var(--muted-foreground)] hover:text-[var(--foreground)] hover:bg-[var(--muted)]/50",
        isMainAction && "text-green-500 hover:text-green-600 italic",
      )}
      onClick={(e) => {
        e.stopPropagation();
        onSelect();
      }}
    >
      {isMainAction ? (
        <RefreshCw size={14} className="opacity-0 w-0" /> // spacer hack if needed, or just icon
      ) : (
        <TableIcon
          size={14}
          className={cn(isSelected && "text-[var(--primary)]")}
        />
      )}
      {isMainAction && <Plus size={14} />}
      <span className="truncate">{tableName}</span>
    </div>
  );
}

// Pagination constants
const ITEMS_PER_PAGE = 50;

function DatabaseParamsNode({
  dbName,
  sourceId,
  isSelected,
  selectedTable,
  onSelectTable,
  onCreateTable,
}: {
  dbName: string;
  sourceId?: string;
  isSelected: boolean;
  selectedTable: string | null;
  onSelectTable: (table: string) => void;
  onCreateTable: () => void;
}) {
  // This component handles the "Tables" listing for a specific database
  const [localFilter, setLocalFilter] = useState("");
  const [page, setPage] = useState(1);

  const effectiveFilter = localFilter;

  const { data: tables = [], isLoading, isError, error } = useTables(dbName, sourceId);

  const filteredTables = (tables || []).filter((t) =>
    t.name?.toLowerCase().includes(effectiveFilter.toLowerCase()),
  );

  // Pagination Logic
  const totalPages = Math.ceil(filteredTables.length / ITEMS_PER_PAGE);
  const safePage = Math.min(Math.max(1, page), Math.max(1, totalPages));

  // Calculate slice
  const startIndex = (safePage - 1) * ITEMS_PER_PAGE;
  const paginatedTables = filteredTables.slice(
    startIndex,
    startIndex + ITEMS_PER_PAGE,
  );

  return (
    <>
      <div className="border-l border-[var(--border)] ml-5 my-1 pl-1">
        {/* Filter Input */}
        <div className="px-2 py-1 mb-1">
          <div className="relative flex items-center">
            <input
              value={localFilter}
              onChange={(e) => {
                setLocalFilter(e.target.value);
                setPage(1); // Reset to page 1 on filter change
              }}
              className="w-full text-xs h-7 px-2 pr-6 rounded border border-[var(--border)] bg-[var(--background)] focus:outline-none focus:border-[var(--primary)] text-[var(--foreground)] placeholder:text-[var(--muted-foreground)]"
              placeholder="Filter tables..."
              onClick={(e) => e.stopPropagation()}
            />
            {localFilter && (
              <button
                className="absolute right-1 text-[var(--muted-foreground)] hover:text-red-400"
                onClick={(e) => {
                  e.stopPropagation();
                  setLocalFilter("");
                  setPage(1);
                }}
              >
                <span className="text-xs font-bold">x</span>
              </button>
            )}
          </div>
        </div>

        {/* Pagination Controls */}
        {totalPages > 1 && (
          <div
            className="flex items-center justify-end gap-2 px-2 py-1 mb-1"
            onClick={(e) => e.stopPropagation()}
          >
            {/* First Page */}
            <div className="flex gap-1">
              <span
                className={cn(
                  "text-xs tracking-widest cursor-pointer",
                  page > 1
                    ? "text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                    : "text-[var(--muted)] cursor-not-allowed",
                )}
                title="End"
                onClick={() => setPage(1)}
              >
                {"<<"}
              </span>
            </div>

            {/* Previous Page */}
            <div className="flex gap-1">
              <span
                className={cn(
                  "text-xs font-bold cursor-pointer",
                  page > 1
                    ? "text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                    : "text-[var(--muted)] cursor-not-allowed",
                )}
                title="Previous"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                {"<"}
              </span>
            </div>

            {/* Page Selector */}
            <div className="flex items-center gap-1">
              <select
                value={safePage}
                onChange={(e) => setPage(Number(e.target.value))}
                className="h-6 text-xs border border-[var(--border)] rounded bg-[var(--card)] px-1"
              >
                {Array.from({ length: totalPages }, (_, i) => i + 1).map(
                  (p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ),
                )}
              </select>
            </div>

            {/* Next Page */}
            <div className="flex gap-1">
              <span
                className={cn(
                  "text-xs font-bold cursor-pointer",
                  page < totalPages
                    ? "text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                    : "text-[var(--muted)] cursor-not-allowed",
                )}
                title="Next"
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              >
                {">"}
              </span>
            </div>

            {/* Last Page */}
            <div className="flex gap-1">
              <span
                className={cn(
                  "text-xs tracking-widest cursor-pointer",
                  page < totalPages
                    ? "text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                    : "text-[var(--muted)] cursor-not-allowed",
                )}
                title="End"
                onClick={() => setPage(totalPages)}
              >
                {">>"}
              </span>
            </div>
          </div>
        )}

        {/* New Table Action */}
        <TableNode
          tableName="New"
          isSelected={false}
          onSelect={onCreateTable}
          isMainAction
        />

        {isLoading && (
          <div className="px-6 py-2">
            <RefreshCw
              size={12}
              className="animate-spin text-[var(--muted-foreground)]"
            />
          </div>
        )}

        {paginatedTables.map((t) => (
          <TableNode
            key={t.name}
            tableName={t.name}
            isSelected={isSelected && selectedTable === t.name}
            onSelect={() => onSelectTable(t.name)}
          />
        ))}
        {isError && !isLoading && (
          <div className="px-6 py-2 text-xs text-red-400">
            {error instanceof Error ? error.message : "Failed to load tables"}
          </div>
        )}
        {!isError && filteredTables.length === 0 && !isLoading && (
          <div className="px-6 py-2 text-xs text-[var(--muted-foreground)] italic">
            {effectiveFilter ? "No matching tables" : "No tables"}
          </div>
        )}
      </div>
    </>
  );
}

function RecursiveTreeNode({
  node,
  level = 0,
  selectedDb,
  selectedTable,
  activeSourceId,
  onSelectDb,
  onSelectTable,
  onCreateTable,
  onCloneDatabase,
  onContextMenu,
  filter,
}: {
  node: TreeNode;
  level?: number;
  selectedDb: string | null;
  selectedTable: string | null;
  activeSourceId?: string;
  onSelectDb: (db: string, sourceId?: string) => void;
  onSelectTable: (db: string, table: string, sourceId?: string) => void;
  onCreateTable: (db: string, sourceId?: string) => void;
  onCloneDatabase: (db: string, sourceId?: string) => void;
  onContextMenu: (e: React.MouseEvent, dbName: string, sourceId?: string) => void;
  filter: string;
}) {
  const [expanded, setExpanded] = useState(false);

  const hasChildren = Object.keys(node.children).length > 0;

  const matchesSource =
    !node.sourceId || !activeSourceId || node.sourceId === activeSourceId;
  const isSelectedPath = node.isDatabase && selectedDb === node.fullName && matchesSource;
  const isParentOfSelected =
    !!selectedDb &&
    ((!node.isDatabase && selectedDb === node.fullName) ||
      selectedDb.startsWith(node.fullName + "_"));

  // Auto-expand grouped folders when filtering or when a child DB is selected
  useEffect(() => {
    if (isParentOfSelected || (!!filter && !node.isDatabase && hasChildren)) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setExpanded(true);
    }
  }, [isParentOfSelected, filter, node.isDatabase, hasChildren]);

  const isExpanded = expanded;

  const toggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    setExpanded(!expanded);
  };

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (node.isDatabase) {
      onSelectDb(node.fullName, node.sourceId);

      if (expanded && isSelectedPath) {
        setExpanded(false);
      } else if (!expanded) {
        setExpanded(true);
      }
    } else {
      setExpanded(!expanded);
    }
  };

  const tooltip = node.isDatabase
    ? [
        node.fullName,
        node.sourceName,
        node.driver ? node.driver.toUpperCase() : null,
        node.tables !== undefined ? `${node.tables} tables` : null,
      ]
        .filter(Boolean)
        .join(" · ")
    : node.fullName;

  return (
    <div style={{ marginLeft: level > 0 ? "12px" : "0px" }}>
      <div
        className={cn(
          "flex items-center gap-2 px-3 py-1.5 text-sm cursor-pointer transition-colors group rounded-sm mb-0.5",
          isSelectedPath
            ? "bg-[var(--muted)] text-[var(--foreground)] font-medium"
            : "text-[var(--muted-foreground)] hover:text-[var(--foreground)] hover:bg-[var(--muted)]/30",
        )}
        title={tooltip}
        onClick={handleClick}
        onContextMenu={(e) => {
          if (node.isDatabase) {
            onContextMenu(e, node.fullName, node.sourceId);
          }
        }}
      >
        {/* Expand Icon */}
        <button
          onClick={toggle}
          className="p-0.5 rounded hover:bg-[var(--muted)]/80 text-[var(--muted-foreground)]"
        >
          {(hasChildren || node.isDatabase) &&
            (isExpanded ? (
              <ChevronDown size={14} />
            ) : (
              <ChevronRight size={14} />
            ))}
          {!(hasChildren || node.isDatabase) && (
            <span className="w-3.5 h-3.5 inline-block" />
          )}
        </button>

        {/* Type Icon */}
        {node.isDatabase ? (
          <DatabaseIcon
            size={14}
            className={cn(
              "shrink-0",
              node.driver === "postgres" ? "text-indigo-400" : "text-blue-400",
              isSelectedPath && "fill-blue-400/20",
            )}
          />
        ) : (
          <Folder size={14} className="text-yellow-500/80 shrink-0" />
        )}

        <span className="flex-1 truncate">{node.name}</span>

        {/* Table count badge (hidden on hover to make room for clone action) */}
        {node.isDatabase && node.tables !== undefined && (
          <span className="text-[10px] font-mono text-[var(--muted-foreground)]/80 group-hover:hidden shrink-0">
            {node.tables}
          </span>
        )}

        {/* Group child count badge */}
        {!node.isDatabase && hasChildren && (
          <span className="text-[10px] font-mono text-[var(--muted-foreground)]/70 shrink-0">
            {Object.keys(node.children).length}
          </span>
        )}

        {/* Clone Action */}
        {node.isDatabase && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onCloneDatabase(node.fullName, node.sourceId);
            }}
            className="hidden group-hover:inline-flex p-1 text-[var(--muted-foreground)] hover:text-[var(--primary)] transition-opacity"
            title="Clone Database"
          >
            <Copy size={12} />
          </button>
        )}
      </div>

      {isExpanded && (
        <div>
          {/* Render children groups/databases first so sibling prefix DBs are never buried under tables */}
          {hasChildren && (
            <div className="border-l border-[var(--border)] ml-2 pl-1">
              {Object.values(node.children)
                .sort((a, b) => a.name.localeCompare(b.name))
                .map((child) => (
                  <RecursiveTreeNode
                    key={child.key}
                    node={child}
                    level={0}
                    selectedDb={selectedDb}
                    selectedTable={selectedTable}
                    activeSourceId={activeSourceId}
                    onSelectDb={onSelectDb}
                    onSelectTable={onSelectTable}
                    onCreateTable={onCreateTable}
                    onCloneDatabase={onCloneDatabase}
                    onContextMenu={onContextMenu}
                    filter={filter}
                  />
                ))}
            </div>
          )}

          {/* If this node represents a real database, show its tables */}
          {node.isDatabase && (
            <DatabaseParamsNode
              dbName={node.fullName}
              sourceId={node.sourceId}
              isSelected={isSelectedPath}
              selectedTable={selectedTable}
              onSelectTable={(t) => onSelectTable(node.fullName, t, node.sourceId)}
              onCreateTable={() => onCreateTable(node.fullName, node.sourceId)}
            />
          )}
        </div>
      )}
    </div>
  );
}

export function DatabaseTree({
  selectedDb,
  selectedTable,
  activeSourceId,
  serverLabel,
  connected,
  onSelectDb,
  onSelectTable,
  onCreateTable,
  onCloneDatabase,
  onRenameDatabase,
  onCreateDatabase,
  onDeleteDatabase,
}: DatabaseTreeProps) {
  const {
    data: catalog = [],
    isLoading: catalogLoading,
    isError: catalogError,
    error: catalogErrObj,
    refetch: refetchCatalog,
  } = useDatabaseCatalog();
  const {
    data: activeDatabases = [],
    isLoading: activeLoading,
    isError: activeError,
    error: activeErrObj,
    refetch: refetchActive,
  } = useDatabases();
  const renameMutation = useRenameDatabaseMutation();
  const [searchTerm, setSearchTerm] = useState("");
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    dbName: string | null;
    sourceId?: string;
  } | null>(null);

  const allDatabases: TreeDatabaseInput[] = useMemo(() => {
    if (catalog.length > 0) {
      return catalog;
    }
    if (catalogLoading) {
      return [];
    }
    return activeDatabases.map((db) => ({
      name: db.name,
      tables: db.tables,
      source_id: activeSourceId,
      source_name: serverLabel,
    }));
  }, [catalog, catalogLoading, activeDatabases, activeSourceId, serverLabel]);

  const isLoading = (catalogLoading && catalog.length === 0) || (catalogError && activeLoading);
  const isError = catalog.length === 0 && activeDatabases.length === 0 && (catalogError || activeError);
  const error = catalogErrObj || activeErrObj;

  const searchFilter = searchTerm.trim().toLowerCase();

  const filteredDatabases = useMemo(() => {
    if (!searchFilter) return allDatabases;
    return allDatabases.filter(
      (db) =>
        db.name.toLowerCase().includes(searchFilter) ||
        (db.source_name || "").toLowerCase().includes(searchFilter),
    );
  }, [allDatabases, searchFilter]);

  const tree = useMemo(
    () => buildDatabaseTree(filteredDatabases),
    [filteredDatabases],
  );

  const handleRefresh = () => {
    refetchCatalog();
    refetchActive();
  };

  const handleContextMenu = (
    e: React.MouseEvent,
    dbName: string | null,
    sourceId?: string,
  ) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, dbName, sourceId });
  };

  const handleRename = (oldName: string, sourceId?: string) => {
    if (onRenameDatabase) {
      onRenameDatabase(oldName, sourceId);
      return;
    }
    const newName = prompt("Enter new database name:", oldName);
    if (newName && newName !== oldName) {
      renameMutation.mutate({ oldName, newName });
    }
  };

  const headerTitle =
    allDatabases.length > 0
      ? `Databases (${allDatabases.length})`
      : serverLabel || "Databases";

  return (
    <div
      className="flex flex-col h-full bg-[var(--card)] border-r border-[var(--border)]"
      onContextMenu={(e) => handleContextMenu(e, null)}
    >
      {/* Header */}
      <div className="p-3 border-b border-[var(--border)] flex flex-col gap-2 bg-[var(--muted)]/20">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-sm font-semibold min-w-0">
            <Server size={16} className="text-purple-400 shrink-0" />
            <span className="truncate" title={serverLabel || headerTitle}>
              {headerTitle}
            </span>
            {connected !== undefined && (
              <span
                className={cn(
                  "h-2 w-2 rounded-full shrink-0",
                  connected || allDatabases.length > 0
                    ? "bg-emerald-500"
                    : "bg-red-500",
                )}
                title={
                  connected || allDatabases.length > 0
                    ? "Connected"
                    : "Disconnected"
                }
              />
            )}
          </div>
          <div className="flex gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0 text-green-500 hover:text-green-600 hover:bg-green-500/10"
              onClick={onCreateDatabase}
              title="New Database"
            >
              <Plus size={14} />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0"
              onClick={handleRefresh}
              title="Refresh All Databases"
            >
              <RefreshCw size={12} />
            </Button>
          </div>
        </div>
        <div className="relative">
          <Search
            size={12}
            className="absolute left-2 top-2.5 text-[var(--muted-foreground)]"
          />
          <Input
            placeholder="Search databases or containers..."
            className="h-8 pl-8 bg-[var(--background)]"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-y-auto py-2 px-1">
        {isLoading && (
          <div className="flex justify-center p-4">
            <RefreshCw className="animate-spin text-[var(--muted-foreground)]" />
          </div>
        )}

        {tree.map((node) => (
          <RecursiveTreeNode
            key={node.key}
            node={node}
            level={0}
            selectedDb={selectedDb}
            selectedTable={selectedTable}
            activeSourceId={activeSourceId}
            onSelectDb={onSelectDb}
            onSelectTable={onSelectTable}
            onCreateTable={onCreateTable}
            onCloneDatabase={onCloneDatabase}
            onContextMenu={handleContextMenu}
            filter={searchTerm}
          />
        ))}

        {!isLoading && isError && (
          <div className="text-center py-8 text-red-400 text-xs px-3">
            {error instanceof Error ? error.message : "Failed to load databases"}
          </div>
        )}

        {!isLoading && !isError && tree.length === 0 && (
          <div className="text-center py-8 text-[var(--muted-foreground)] text-xs">
            No databases found
          </div>
        )}
      </div>

      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          items={
            contextMenu.dbName
              ? [
                  {
                    label: "Open",
                    onClick: () =>
                      onSelectDb(contextMenu.dbName!, contextMenu.sourceId),
                  },
                  {
                    label: "New Table",
                    icon: <Plus size={14} />,
                    onClick: () =>
                      onCreateTable(contextMenu.dbName!, contextMenu.sourceId),
                  },
                  {
                    label: "Clone Database",
                    icon: <Copy size={14} />,
                    onClick: () =>
                      onCloneDatabase(contextMenu.dbName!, contextMenu.sourceId),
                  },
                  {
                    label: "Rename Database",
                    icon: <Edit2 size={14} />,
                    onClick: () =>
                      handleRename(contextMenu.dbName!, contextMenu.sourceId),
                  },
                  {
                    label: "Delete Database",
                    icon: <Trash2 size={14} />,
                    variant: "danger",
                    onClick: () =>
                      onDeleteDatabase(contextMenu.dbName!, contextMenu.sourceId),
                  },
                ]
              : [
                  {
                    label: "Create New Database",
                    icon: <Plus size={14} />,
                    onClick: onCreateDatabase,
                  },
                  {
                    label: "Refresh List",
                    icon: <RefreshCw size={14} />,
                    onClick: handleRefresh,
                  },
                ]
          }
        />
      )}
    </div>
  );
}
