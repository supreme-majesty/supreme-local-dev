import { useState } from "react";
import {
  Database as DatabaseIcon,
  Table as TableIcon,
  ChevronRight,
  ChevronDown,
  Server,
  RefreshCw,
  MoreVertical,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useDatabases, useTables } from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";

interface DatabaseTreeProps {
  selectedDb: string | null;
  selectedTable: string | null;
  onSelectDb: (db: string) => void;
  onSelectTable: (db: string, table: string) => void;
}

function TableNode({
  tableName,
  isSelected,
  onSelect,
}: {
  tableName: string;
  isSelected: boolean;
  onSelect: () => void;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 px-6 py-1.5 text-sm cursor-pointer transition-colors",
        isSelected
          ? "bg-[var(--primary)]/10 text-[var(--primary)] border-r-2 border-[var(--primary)]"
          : "text-[var(--muted-foreground)] hover:text-[var(--foreground)] hover:bg-[var(--muted)]/50"
      )}
      onClick={(e) => {
        e.stopPropagation();
        onSelect();
      }}
    >
      <TableIcon
        size={14}
        className={cn(isSelected && "text-[var(--primary)]")}
      />
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
}: {
  name: string;
  isSelected: boolean;
  selectedTable: string | null;
  onSelectDb: () => void;
  onSelectTable: (table: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const { data: tables = [], isLoading } = useTables(expanded ? name : null);

  const toggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    setExpanded(!expanded);
    if (!expanded) {
      onSelectDb();
    }
  };

  const handleClick = () => {
    onSelectDb();
    setExpanded(true);
  };

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
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
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

      {expanded && (
        <div className="border-l border-[var(--border)] ml-5 my-1">
          {tables.map((t) => (
            <TableNode
              key={t.name}
              tableName={t.name}
              isSelected={isSelected && selectedTable === t.name}
              onSelect={() => onSelectTable(t.name)}
            />
          ))}
          {tables.length === 0 && !isLoading && (
            <div className="px-6 py-2 text-xs text-[var(--muted-foreground)] italic">
              No tables
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
}: DatabaseTreeProps) {
  const { data: databases = [], isLoading, refetch } = useDatabases();

  return (
    <div className="flex flex-col h-full bg-[var(--card)] border-r border-[var(--border)]">
      {/* Header */}
      <div className="p-3 border-b border-[var(--border)] flex items-center justify-between bg-[var(--muted)]/20">
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

      {/* Tree */}
      <div className="flex-1 overflow-y-auto py-2">
        {isLoading && (
          <div className="flex justify-center p-4">
            <RefreshCw className="animate-spin text-[var(--muted-foreground)]" />
          </div>
        )}

        {databases.map((db) => (
          <DatabaseNode
            key={db}
            name={db}
            isSelected={selectedDb === db}
            selectedTable={selectedDb === db ? selectedTable : null}
            onSelectDb={() => onSelectDb(db)}
            onSelectTable={(table) => onSelectTable(db, table)}
          />
        ))}
      </div>
    </div>
  );
}
