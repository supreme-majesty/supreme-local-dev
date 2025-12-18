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

function DatabaseNode({
  name,
  isSelected,
  selectedTable,
  onSelectDb,
  onSelectTable,
  onCreateTable,
  filter,
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
  const { data: tables = [], isLoading } = useTables(
    expanded || filter ? name : null
  ); // Auto-fetch if filtering

  // Auto-expand if filter is active and matches tables (or if simpler, just always if filtered)

  // Actually, we want to expand if we have a filter to show matching tables.
  // But we also need to respect user toggle?
  // Standard tree filter behavior: if filter matches children, expand.

  const filteredTables = (tables || []).filter((t) =>
    t.name.toLowerCase().includes(filter.toLowerCase())
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
    expanded || (filter.length > 0 && filteredTables.length > 0);

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
        <div className="border-l border-[var(--border)] ml-5 my-1">
          {/* New Table Action - Only show if NO filter or if "new" matches filter (optional) */}
          {!filter && (
            <TableNode
              tableName="New"
              isSelected={false}
              onSelect={onCreateTable}
              isMainAction
            />
          )}

          {filteredTables.map((t) => (
            <TableNode
              key={t.name}
              tableName={t.name}
              isSelected={isSelected && selectedTable === t.name}
              onSelect={() => onSelectTable(t.name)}
            />
          ))}
          {filteredTables.length === 0 && !isLoading && (
            <div className="px-6 py-2 text-xs text-[var(--muted-foreground)] italic">
              {filter ? "No matching tables" : "No tables"}
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

  const filteredDatabases = (databases || []).filter(
    (db) => db.name.toLowerCase().includes(searchTerm.toLowerCase())
    // Note: We might want to also include DBs that have matching tables,
    // but we can't efficiently know that without fetching all tables.
    // For now, strict DB name match OR just rely on the user opening the DB
    // where they expect matches?
    // Actually, common pattern is: show DB if name matches.
    // If name doesn't match, we hide it.
    // BUT what if a user searches for a table name? They expect parent DB to show.
    // That requires fetching tables. Let's stick to DB name filtering + Table name filtering within filtered DBs for now to avoid n+1 fetch storm.
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
