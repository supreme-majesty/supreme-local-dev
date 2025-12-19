import { useState } from "react";
import { useTables } from "@/hooks/use-database";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { formatBytes } from "@/lib/utils";
import {
  Search,
  Trash2,
  Eye,
  FileInput,
  Minimize2,
  Table as TableIcon,
} from "lucide-react";

interface DatabaseStructureProps {
  database: string;
  onSelectTable: (table: string) => void; // For Browse/Structure/Search/Insert
  onDropTable: (table: string) => void;
  onEmptyTable: (table: string) => void;
}

export function DatabaseStructure({
  database,
  onSelectTable,
  onDropTable,
  onEmptyTable,
}: DatabaseStructureProps) {
  const { data: tables = [], isLoading } = useTables(database);
  const [filter, setFilter] = useState("");

  const filteredTables = tables.filter((t) =>
    t.name.toLowerCase().includes(filter.toLowerCase())
  );

  if (isLoading) {
    return (
      <div className="p-8 text-center text-[var(--muted-foreground)]">
        Loading tables...
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-[var(--background)]">
      {/* Filter Bar */}
      <div className="p-4 border-b border-[var(--border)] bg-[var(--muted)]/20">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">Filters</span>
            <div className="relative">
              <span className="absolute left-3 top-2.5 text-xs text-[var(--muted-foreground)]">
                Containing the word:
              </span>
              <Input
                className="pl-32 h-9 w-[300px] bg-[var(--background)]"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
              />
            </div>
          </div>
        </div>
      </div>

      {/* Table List */}
      <div className="flex-1 overflow-auto p-4">
        <div className="border border-[var(--border)] rounded-sm bg-[var(--card)]">
          <table className="w-full text-sm text-left">
            <thead className="bg-[var(--muted)]/50 text-xs font-medium text-[var(--muted-foreground uppercase">
              <tr className="border-b border-[var(--border)]">
                <th className="w-8 p-2 text-center">
                  <input type="checkbox" />
                </th>
                <th className="p-2 font-bold text-[var(--foreground)]">
                  Table
                </th>
                <th
                  className="p-2 font-bold text-[var(--foreground)]"
                  colSpan={6}
                >
                  Action
                </th>
                <th className="p-2 text-right">Rows</th>
                <th className="p-2">Type</th>
                <th className="p-2">Collation</th>
                <th className="p-2 text-right">Size</th>
                <th className="p-2 text-right">Overhead</th>
              </tr>
            </thead>
            <tbody>
              {filteredTables.map((t) => (
                <tr
                  key={t.name}
                  className="border-b border-[var(--border)] hover:bg-[var(--muted)]/20 group"
                >
                  <td className="p-2 text-center">
                    <input type="checkbox" />
                  </td>
                  <td
                    className="p-2 font-medium text-[var(--primary)] hover:underline cursor-pointer"
                    onClick={() => onSelectTable(t.name)}
                  >
                    {t.name}
                  </td>
                  <td className="p-1 w-8">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 gap-1 px-1 text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                      onClick={() => onSelectTable(t.name)}
                    >
                      <Eye size={12} /> <span className="text-xs">Browse</span>
                    </Button>
                  </td>
                  <td className="p-1 w-8">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 gap-1 px-1 text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                      onClick={() => onSelectTable(t.name)}
                    >
                      <TableIcon size={12} />{" "}
                      <span className="text-xs">Structure</span>
                    </Button>
                  </td>
                  <td className="p-1 w-8">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 gap-1 px-1 text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                    >
                      <Search size={12} />{" "}
                      <span className="text-xs">Search</span>
                    </Button>
                  </td>
                  <td className="p-1 w-8">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 gap-1 px-1 text-[var(--muted-foreground)] hover:text-[var(--primary)]"
                    >
                      <FileInput size={12} />{" "}
                      <span className="text-xs">Insert</span>
                    </Button>
                  </td>
                  <td className="p-1 w-8">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 gap-1 px-1 text-[var(--muted-foreground)] hover:text-red-400"
                      onClick={() => onEmptyTable(t.name)}
                    >
                      <Minimize2 size={12} />{" "}
                      <span className="text-xs">Empty</span>
                    </Button>
                  </td>
                  <td className="p-1 w-8">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 gap-1 px-1 text-[var(--muted-foreground)] hover:text-red-500"
                      onClick={() => onDropTable(t.name)}
                    >
                      <Trash2 size={12} /> <span className="text-xs">Drop</span>
                    </Button>
                  </td>

                  <td className="p-2 text-right font-mono text-xs">
                    {t.row_count}
                  </td>
                  <td className="p-2 text-xs text-[var(--muted-foreground)]">
                    {t.engine}
                  </td>
                  <td className="p-2 text-xs text-[var(--muted-foreground)]">
                    {t.collation}
                  </td>
                  <td className="p-2 text-right font-mono text-xs">
                    {formatBytes(t.size)}
                  </td>
                  <td className="p-2 text-right font-mono text-xs text-[var(--muted-foreground)]">
                    {t.overhead > 0 ? formatBytes(t.overhead) : "-"}
                  </td>
                </tr>
              ))}
              {filteredTables.length === 0 && (
                <tr>
                  <td
                    colSpan={13}
                    className="p-8 text-center text-[var(--muted-foreground)] italic"
                  >
                    No tables found matching your filter.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          <div className="p-2 text-xs text-[var(--muted-foreground)] border-t border-[var(--border)] bg-[var(--muted)]/20">
            {filteredTables.length} tables
          </div>
        </div>
      </div>
    </div>
  );
}
