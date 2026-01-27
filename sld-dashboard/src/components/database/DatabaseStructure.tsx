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
import { Checkbox } from "@/components/ui/Checkbox";

interface DatabaseStructureProps {
  database: string;
  onSelectTable: (table: string) => void;
  onDropTable: (table: string) => void;
  onEmptyTable: (table: string) => void;
  onBulkDrop: (tables: string[]) => void;
  onBulkEmpty: (tables: string[]) => void;
  onBulkAction: (tables: string[], action: string) => void;
}

export function DatabaseStructure({
  database,
  onSelectTable,
  onDropTable,
  onEmptyTable,
  onBulkDrop,
  onBulkEmpty,
  onBulkAction,
}: DatabaseStructureProps) {
  const { data: tables = [], isLoading } = useTables(database);
  const [filter, setFilter] = useState("");
  const [selectedTables, setSelectedTables] = useState<Set<string>>(new Set());

  const filteredTables = tables.filter((t) =>
    t.name.toLowerCase().includes(filter.toLowerCase())
  );

  const allSelected =
    filteredTables.length > 0 &&
    filteredTables.every((t) => selectedTables.has(t.name));

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedTables(new Set(filteredTables.map((t) => t.name)));
    } else {
      setSelectedTables(new Set());
    }
  };

  const handleSelectTable = (table: string, checked: boolean) => {
    const newSet = new Set(selectedTables);
    if (checked) newSet.add(table);
    else newSet.delete(table);
    setSelectedTables(newSet);
  };

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
              <Input
                className="h-9 w-[300px] bg-[var(--background)]"
                placeholder="Search tables..."
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
                  <Checkbox
                    checked={allSelected}
                    onChange={(e) => handleSelectAll(e.target.checked)}
                  />
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
                    <Checkbox
                      checked={selectedTables.has(t.name)}
                      onChange={(e) =>
                        handleSelectTable(t.name, e.target.checked)
                      }
                    />
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
          <div className="p-2 text-xs text-[var(--muted-foreground)] border-t border-[var(--border)] bg-[var(--muted)]/20 flex items-center justify-between">
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <Checkbox
                  checked={allSelected}
                  onChange={(e) => handleSelectAll(e.target.checked)}
                />
                <span>Check all</span>
              </label>

              <select
                className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-sm text-[var(--foreground)]"
                value=""
                onChange={(e) => {
                  const val = e.target.value;
                  if (!val) return;
                  const tables = Array.from(selectedTables);
                  if (val === "drop") onBulkDrop(tables);
                  else if (val === "empty") onBulkEmpty(tables);
                  else onBulkAction(tables, val);
                  setSelectedTables(new Set());
                  e.target.value = "";
                }}
                disabled={selectedTables.size === 0}
              >
                <option value="">With selected:</option>
                <option value="copy">Copy table</option>
                <option value="show_create">Show create</option>
                <option value="export">Export</option>
                <optgroup label="Delete data or table">
                  <option value="empty">Empty</option>
                  <option value="drop">Drop</option>
                </optgroup>
                <optgroup label="Table maintenance">
                  <option value="analyze">Analyze table</option>
                  <option value="check">Check table</option>
                  <option value="checksum">Checksum table</option>
                  <option value="optimize">Optimize table</option>
                  <option value="repair">Repair table</option>
                </optgroup>
                <optgroup label="Prefix">
                  <option value="add_prefix">Add prefix to table</option>
                  <option value="replace_prefix">Replace table prefix</option>
                  <option value="copy_with_prefix">
                    Copy table with prefix
                  </option>
                </optgroup>
              </select>
            </div>
            <span>{filteredTables.length} tables</span>
          </div>
        </div>
      </div>
    </div>
  );
}
