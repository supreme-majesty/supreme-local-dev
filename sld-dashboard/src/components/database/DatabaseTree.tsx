import { useState } from "react";
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
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useDatabases, useTables } from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

interface DatabaseTreeProps {
  selectedDb: string | null;
  selectedTable: string | null;
  onSelectDb: (db: string) => void;
  onSelectTable: (db: string, table: string) => void;
  onCreateTable: (db: string) => void;
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
        isMainAction && "text-green-500 hover:text-green-600 italic"
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
const ITEMS_PER_PAGE = 10;

function DatabaseNode({
  name,
  isSelected,
  selectedTable,
  onSelectDb,
  onSelectTable,
  onCreateTable,
  filter: globalFilter,
}: {
  name: string;
  isSelected: boolean;
  selectedTable: string | null;
  onSelectDb: () => void;
  onSelectTable: (table: string) => void;
  onCreateTable: () => void;
  filter: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const [localFilter, setLocalFilter] = useState("");
  const [page, setPage] = useState(1);

  // Combine global filter (from top search) and local filter
  const effectiveFilter = localFilter || globalFilter;

  const { data: tables = [], isLoading } = useTables(
    expanded || effectiveFilter ? name : null
  );

  const filteredTables = (tables || []).filter((t) =>
    t.name?.toLowerCase().includes(effectiveFilter.toLowerCase())
  );

  // Pagination Logic
  const totalPages = Math.ceil(filteredTables.length / ITEMS_PER_PAGE);
  const safePage = Math.min(Math.max(1, page), Math.max(1, totalPages));

  // Calculate slice
  const startIndex = (safePage - 1) * ITEMS_PER_PAGE;
  const paginatedTables = filteredTables.slice(
    startIndex,
    startIndex + ITEMS_PER_PAGE
  );

  const toggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    setExpanded(!expanded);
    if (!expanded) {
      onSelectDb();
    }
  };

  const handleClick = () => {
    onSelectDb();
    setExpanded(!expanded);
  };

  // If effective expansion state depends on filter
  const isExpanded =
    expanded || (globalFilter.length > 0 && filteredTables.length > 0);

  return (
    <div>
      <div
        className={cn(
          "flex items-center gap-2 px-3 py-2 text-sm cursor-pointer transition-colors group",
          isSelected && !selectedTable
            ? "bg-[var(--muted)] text-[var(--foreground)]"
            : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
        )}
        onClick={handleClick}
      >
        <button
          onClick={toggle}
          className="p-0.5 rounded hover:bg-[var(--muted)]/80"
        >
          {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <DatabaseIcon size={14} className="text-blue-400" />
        <span className="flex-1 truncate font-medium">{name}</span>
        {isLoading && (
          <RefreshCw
            size={12}
            className="animate-spin text-[var(--muted-foreground)]"
          />
        )}
      </div>

      {isExpanded && (
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
                placeholder="Type to filter these, Enter"
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
              className="flex items-center gap-2 px-2 py-1 mb-1"
              onClick={(e) => e.stopPropagation()}
            >
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
                    )
                  )}
                </select>
              </div>
              <div className="flex gap-1">
                {/* Only show simplified arrows like >> if strictly matching image, but arrows represent prev/next usually */}
                {/* If image had "1 v >>>" style: */}
                <span
                  className="text-xs text-[var(--muted-foreground)] tracking-widest cursor-pointer hover:text-[var(--primary)]"
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                >
                  &gt;&gt;&gt;
                </span>
              </div>
            </div>
          )}

          {/* New Table Action - Always visible */}
          <TableNode
            tableName="New"
            isSelected={false}
            onSelect={onCreateTable}
            isMainAction
          />

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
}: DatabaseTreeProps) {
  const { data: databases = [], isLoading, refetch } = useDatabases();
  const [searchTerm, setSearchTerm] = useState("");

  const filteredDatabases = (databases || []).filter((db) =>
    db.name?.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="flex flex-col h-full bg-[var(--card)] border-r border-[var(--border)]">
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
      <div className="flex-1 overflow-y-auto py-2">
        {isLoading && (
          <div className="flex justify-center p-4">
            <RefreshCw className="animate-spin text-[var(--muted-foreground)]" />
          </div>
        )}

        {filteredDatabases.map((db) => (
          <DatabaseNode
            key={db.name}
            name={db.name}
            isSelected={selectedDb === db.name}
            selectedTable={selectedDb === db.name ? selectedTable : null}
            onSelectDb={() => onSelectDb(db.name)}
            onSelectTable={(table) => onSelectTable(db.name, table)}
            onCreateTable={() => onCreateTable(db.name)}
            filter={searchTerm}
          />
        ))}
      </div>
    </div>
  );
}
