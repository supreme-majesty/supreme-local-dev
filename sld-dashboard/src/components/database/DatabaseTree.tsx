import { useState, useEffect, useMemo } from "react";
import {
  Database as DatabaseIcon,
  Table as TableIcon,
  ChevronRight,
  ChevronDown,
  Server,
  RefreshCw,
  MoreVertical,
  Plus,
  Search,
  Folder,
  Copy,
  Trash2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useDatabases, useTables } from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ContextMenu } from "@/components/ui/ContextMenu";
import { buildDatabaseTree, type TreeNode } from "./tree-utils";

interface DatabaseTreeProps {
  selectedDb: string | null;
  selectedTable: string | null;
  onSelectDb: (db: string) => void;
  onSelectTable: (db: string, table: string) => void;
  onCreateTable: (db: string) => void;
  onCloneDatabase: (db: string) => void;
  onCreateDatabase: () => void;
  onDeleteDatabase: (db: string) => void;
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
  isSelected, // eslint-disable-line @typescript-eslint/no-unused-vars
  selectedTable,
  onSelectTable,
  onCreateTable,
  filter,
}: {
  dbName: string;
  isSelected: boolean;
  selectedTable: string | null;
  onSelectTable: (table: string) => void;
  onCreateTable: () => void;
  filter: string;
}) {
  // This component handles the "Tables" listing for a specific database
  const [localFilter, setLocalFilter] = useState("");
  const [page, setPage] = useState(1);

  // Combine global filter (from top search) and local filter
  const effectiveFilter = localFilter || filter;

  // Since this component is only rendered when the parent node is expanded (or it's a root match),
  // we assume we should fetch.
  // Optimization: If we want to defer fetching until expanded, the Parent controls that.
  // The Parent only renders this <DatabaseParamsNode> when it is expanded.
  // So we can always fetch here.

  const { data: tables = [], isLoading } = useTables(dbName);

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
        {filteredTables.length === 0 && !isLoading && (
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
  onSelectDb: (db: string) => void;
  onSelectTable: (db: string, table: string) => void;
  onCreateTable: (db: string) => void;
  onCloneDatabase: (db: string) => void;
  onContextMenu: (e: React.MouseEvent, dbName: string) => void;
  filter: string;
}) {
  const [expanded, setExpanded] = useState(false);

  // If the node corresponds to the currently selected DB, let's auto-expand the tree path?
  // Only if we haven't manually toggled it? For now, let's keep it simple.

  const hasChildren = Object.keys(node.children).length > 0;

  // If selectedDB starts with this node's fullName + '_', we should probably be expanded
  const isSelectedPath = selectedDb === node.fullName;
  const isParentOfSelected = selectedDb?.startsWith(node.fullName + "_");
  const isMatchingFilter = !!filter && node.fullName.includes(filter);

  // Auto-expand if we are in the path of the selected DB or filtering matches
  // Use effect so user can collapse it manually afterwards
  useEffect(() => {
    if (isParentOfSelected || isMatchingFilter) {
      setExpanded(true);
    }
  }, [isParentOfSelected, isMatchingFilter]);

  const isExpanded = expanded;

  // Toggle handler
  const toggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    setExpanded(!expanded);
    // If it is a real database, selecting it also
    if (node.isDatabase && !expanded) {
      onSelectDb(node.fullName);
    }
  };

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (node.isDatabase) {
      onSelectDb(node.fullName);

      // If already expanded and selected, toggle close
      if (expanded && selectedDb === node.fullName) {
        setExpanded(false);
      } else if (!expanded) {
        setExpanded(true);
      }
    } else {
      // Just a group folder, toggle it
      setExpanded(!expanded);
    }
  };

  return (
    <div style={{ marginLeft: level > 0 ? "12px" : "0px" }}>
      <div
        className={cn(
          "flex items-center gap-2 px-3 py-1.5 text-sm cursor-pointer transition-colors group rounded-sm mb-0.5",
          isSelectedPath
            ? "bg-[var(--muted)] text-[var(--foreground)] font-medium"
            : "text-[var(--muted-foreground)] hover:text-[var(--foreground)] hover:bg-[var(--muted)]/30",
        )}
        onClick={handleClick}
        onContextMenu={(e) => {
          if (node.isDatabase) {
            onContextMenu(e, node.fullName);
          }
        }}
      >
        {/* Expand Icon */}
        <button
          onClick={toggle}
          className="p-0.5 rounded hover:bg-[var(--muted)]/80 text-[var(--muted-foreground)]"
        >
          {/* Always show chevron if functionality exists (children or tables) */}
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
              "text-blue-400",
              isSelectedPath && "fill-blue-400/20",
            )}
          />
        ) : (
          <Folder size={14} className="text-yellow-500/80" />
        )}

        <span className="flex-1 truncate">{node.name}</span>

        {/* Clone Action */}
        {node.isDatabase && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onCloneDatabase(node.fullName);
            }}
            className="p-1 text-[var(--muted-foreground)] hover:text-[var(--primary)] opacity-0 group-hover:opacity-100 transition-opacity"
            title="Clone Database"
          >
            <Copy size={12} />
          </button>
        )}
      </div>

      {isExpanded && (
        <div>
          {/* If this node represents a real database, show its tables */}
          {node.isDatabase && (
            <DatabaseParamsNode
              dbName={node.fullName}
              isSelected={isSelectedPath}
              selectedTable={selectedTable}
              onSelectTable={(t) => onSelectTable(node.fullName, t)}
              onCreateTable={() => onCreateTable(node.fullName)}
              filter={filter}
            />
          )}

          {/* Render children groups/databases */}
          {hasChildren && (
            <div className="border-l border-[var(--border)] ml-2 pl-1">
              {Object.values(node.children)
                .sort((a, b) => a.name.localeCompare(b.name)) // Sort children A-Z
                .map((child) => (
                  <RecursiveTreeNode
                    key={child.fullName}
                    node={child}
                    level={0} // We handle indentation via border-l and padding above
                    selectedDb={selectedDb}
                    selectedTable={selectedTable}
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
        </div>
      )}
    </div>
  );
}

export function DatabaseTree({
  selectedDb,
  selectedTable,
  onSelectDb,
  onSelectTable,
  onCreateTable,
  onCloneDatabase,
  onCreateDatabase,
  onDeleteDatabase,
}: DatabaseTreeProps) {
  const { data: databases = [], isLoading, refetch } = useDatabases();
  const [searchTerm, setSearchTerm] = useState("");
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    dbName: string | null; // null means background
  } | null>(null);

  const searchFilter = searchTerm.toLowerCase();

  const filteredDatabases = useMemo(() => {
    if (!searchFilter) return databases;
    return databases.filter((db) =>
      db.name.toLowerCase().includes(searchFilter),
    );
  }, [databases, searchFilter]);

  const tree = useMemo(
    () => buildDatabaseTree(filteredDatabases),
    [filteredDatabases],
  );

  const handleContextMenu = (e: React.MouseEvent, dbName: string | null) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, dbName });
  };

  return (
    <div
      className="flex flex-col h-full bg-[var(--card)] border-r border-[var(--border)]"
      onContextMenu={(e) => handleContextMenu(e, null)}
    >
      {/* Header */}
      <div className="p-3 border-b border-[var(--border)] flex flex-col gap-2 bg-[var(--muted)]/20">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-sm font-semibold">
            <Server size={16} className="text-purple-400" />
            <span>Localhost</span>
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
              onClick={() => refetch()}
            >
              <RefreshCw size={12} />
            </Button>
            <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
              <MoreVertical size={12} />
            </Button>
          </div>
        </div>
        <div className="relative">
          <Search
            size={12}
            className="absolute left-2 top-2.5 text-[var(--muted-foreground)]"
          />
          <Input
            placeholder="Search..."
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
            key={node.fullName}
            node={node}
            level={0}
            selectedDb={selectedDb}
            selectedTable={selectedTable}
            onSelectDb={onSelectDb}
            onSelectTable={onSelectTable}
            onCreateTable={onCreateTable}
            onCloneDatabase={onCloneDatabase}
            onContextMenu={handleContextMenu}
            filter={searchTerm}
          />
        ))}

        {!isLoading && tree.length === 0 && (
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
                    onClick: () => onSelectDb(contextMenu.dbName!),
                  },
                  {
                    label: "New Table",
                    icon: <Plus size={14} />,
                    onClick: () => onCreateTable(contextMenu.dbName!),
                  },
                  {
                    label: "Clone Database",
                    icon: <Copy size={14} />,
                    onClick: () => onCloneDatabase(contextMenu.dbName!),
                  },
                  {
                    label: "Delete Database",
                    icon: <Trash2 size={14} />,
                    variant: "danger",
                    onClick: () => onDeleteDatabase(contextMenu.dbName!),
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
                    onClick: refetch,
                  },
                ]
          }
        />
      )}
    </div>
  );
}
