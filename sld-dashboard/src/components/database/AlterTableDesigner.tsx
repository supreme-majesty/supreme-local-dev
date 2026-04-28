import { useState, useEffect } from "react";
import { Plus, Trash2, Save, Code2, X } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/Card";
import { useTableColumns } from "@/hooks/use-database";

interface AlterTableDesignerProps {
  database: string;
  table: string;
  onCancel: () => void;
  onSave: (query: string) => void;
  isLoading?: boolean;
}

const COLUMN_TYPES = [
  "INT",
  "VARCHAR",
  "TEXT",
  "DATE",
  "DATETIME",
  "TIMESTAMP",
  "BOOLEAN",
  "DECIMAL",
  "FLOAT",
  "DOUBLE",
  "JSON",
  "BLOB",
  "LONGTEXT",
];

export function AlterTableDesigner({
  database,
  table,
  onCancel,
  onSave,
  isLoading,
}: AlterTableDesignerProps) {
  const { data: originalColumns = [], isLoading: loadingCols } = useTableColumns(
    database,
    table
  );
  
  // We'll also fetch indexes in the future for index editing, but start with columns
  // const { data: originalIndexes = [] } = useTableIndexes(database, table);

  const [columns, setColumns] = useState<any[]>([]);
  const [showSQL, setShowSQL] = useState(false);

  useEffect(() => {
    if (originalColumns.length > 0) {
      setColumns(
        originalColumns.map((col: any) => ({
          ...col,
          _originalName: col.name,
          // Extract length if present e.g., "varchar(255)" -> type="VARCHAR", length="255"
          parsedType: col.type.split("(")[0].toUpperCase(),
          parsedLength: col.type.includes("(")
            ? col.type.split("(")[1].replace(")", "")
            : "",
        }))
      );
    }
  }, [originalColumns]);

  const addColumn = () => {
    setColumns([
      ...columns,
      {
        name: "",
        parsedType: "VARCHAR",
        parsedLength: "255",
        default: "",
        nullable: false,
        key: "",
        extra: "",
        _isNew: true,
      },
    ]);
  };

  const removeColumn = (idx: number) => {
    const newCols = [...columns];
    // If it's not new, we should mark it for dropping instead of just removing it from UI
    // But for simplicity, let's add a _dropped flag
    newCols[idx]._dropped = true;
    setColumns(newCols);
  };

  const updateColumn = (idx: number, field: string, value: any) => {
    const newCols = [...columns];
    newCols[idx] = { ...newCols[idx], [field]: value };
    setColumns(newCols);
  };

  const generateSQL = () => {
    const lines: string[] = [];

    columns.forEach((col) => {
      let typeStr = col.parsedType;
      if (
        col.parsedLength &&
        !["DATE", "DATETIME", "TIMESTAMP", "TEXT", "JSON", "BLOB", "LONGTEXT", "BOOLEAN"].includes(
          col.parsedType
        )
      ) {
        typeStr += `(${col.parsedLength})`;
      }

      const nullStr = col.nullable ? "NULL" : "NOT NULL";
      
      let defaultStr = "";
      if (col.default) {
        if (
          col.default.toUpperCase() === "CURRENT_TIMESTAMP" ||
          col.default.toUpperCase() === "NULL" ||
          !isNaN(Number(col.default))
        ) {
          defaultStr = `DEFAULT ${col.default}`;
        } else {
          defaultStr = `DEFAULT '${col.default}'`;
        }
      }

      const extraStr = col.extra ? col.extra : "";

      if (col._dropped) {
        if (!col._isNew) {
          lines.push(`ALTER TABLE \`${table}\` DROP COLUMN \`${col._originalName}\`;`);
        }
      } else if (col._isNew) {
        lines.push(
          `ALTER TABLE \`${table}\` ADD COLUMN \`${col.name}\` ${typeStr} ${nullStr} ${defaultStr} ${extraStr};`
        );
      } else {
        // Compare with original to see if changed
        const orig = originalColumns.find((c: any) => c.name === col._originalName);
        if (orig) {
          const changed =
            col.name !== col._originalName ||
            typeStr.toLowerCase() !== orig.type.toLowerCase() ||
            col.nullable !== orig.nullable ||
            col.default !== orig.default ||
            (col.extra || "") !== (orig.extra || "");

          if (changed) {
            lines.push(
              `ALTER TABLE \`${table}\` CHANGE COLUMN \`${col._originalName}\` \`${col.name}\` ${typeStr} ${nullStr} ${defaultStr} ${extraStr};`
            );
          }
        }
      }
    });

    return lines.join("\n");
  };

  const handleSave = () => {
    const sql = generateSQL();
    if (sql) {
      onSave(sql);
    } else {
      onCancel(); // No changes
    }
  };

  if (loadingCols) {
    return <div className="p-4 text-center">Loading schema...</div>;
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Alter Table</CardTitle>
            <CardDescription>
              Modify structure for table <strong>{table}</strong>.
            </CardDescription>
          </div>
          <Button variant="ghost" onClick={onCancel} className="h-8 w-8 p-0">
            <X size={16} />
          </Button>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex justify-end mb-4">
            <Button
              variant="ghost"
              onClick={() => setShowSQL(!showSQL)}
              className="gap-2"
            >
              <Code2 size={16} /> {showSQL ? "Hide SQL" : "Preview SQL"}
            </Button>
          </div>

          <div className="border rounded-md overflow-hidden bg-[var(--card)]">
            <div className="overflow-x-auto">
              <table className="w-full text-sm text-left">
                <thead className="bg-[var(--muted)]/50 border-b text-xs uppercase text-[var(--muted-foreground)]">
                  <tr>
                    <th className="px-4 py-3 font-medium w-[200px]">Column Name</th>
                    <th className="px-4 py-3 font-medium w-[150px]">Type</th>
                    <th className="px-4 py-3 font-medium w-[100px]">Length</th>
                    <th className="px-4 py-3 font-medium w-[150px]">Default</th>
                    <th className="px-4 py-3 font-medium text-center w-[60px]">Null</th>
                    <th className="px-4 py-3 font-medium text-center w-[60px]">A_I</th>
                    <th className="px-4 py-3 font-medium w-[50px]"></th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {columns.map((col, idx) => {
                    if (col._dropped) return null;
                    return (
                      <tr key={idx} className="group hover:bg-[var(--muted)]/20">
                        <td className="p-2">
                          <Input
                            value={col.name}
                            onChange={(e) => updateColumn(idx, "name", e.target.value)}
                            placeholder="Column name"
                            className="h-8"
                          />
                        </td>
                        <td className="p-2">
                          <select
                            className="w-full h-8 rounded-md border border-[var(--input)] bg-[var(--background)] px-2 text-xs"
                            value={col.parsedType}
                            onChange={(e) => updateColumn(idx, "parsedType", e.target.value)}
                          >
                            {COLUMN_TYPES.map((t) => (
                              <option key={t} value={t}>
                                {t}
                              </option>
                            ))}
                          </select>
                        </td>
                        <td className="p-2">
                          <Input
                            value={col.parsedLength}
                            onChange={(e) => updateColumn(idx, "parsedLength", e.target.value)}
                            placeholder=""
                            className="h-8"
                            disabled={[
                              "DATE",
                              "DATETIME",
                              "TIMESTAMP",
                              "TEXT",
                              "JSON",
                              "BLOB",
                              "LONGTEXT",
                              "BOOLEAN",
                            ].includes(col.parsedType)}
                          />
                        </td>
                        <td className="p-2">
                          <Input
                            value={col.default || ""}
                            onChange={(e) => updateColumn(idx, "default", e.target.value)}
                            placeholder="NULL"
                            className="h-8"
                          />
                        </td>
                        <td className="p-2 text-center">
                          <input
                            type="checkbox"
                            checked={col.nullable}
                            onChange={(e) => updateColumn(idx, "nullable", e.target.checked)}
                            className="rounded border-[var(--input)]"
                          />
                        </td>
                        <td className="p-2 text-center">
                          <input
                            type="checkbox"
                            checked={col.extra?.toUpperCase() === "AUTO_INCREMENT"}
                            onChange={(e) =>
                              updateColumn(idx, "extra", e.target.checked ? "AUTO_INCREMENT" : "")
                            }
                            className="rounded border-[var(--input)]"
                            disabled={
                              !["INT", "BIGINT", "TINYINT"].some((t) =>
                                col.parsedType.includes(t)
                              )
                            }
                          />
                        </td>
                        <td className="p-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0 text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                            onClick={() => removeColumn(idx)}
                          >
                            <Trash2 size={14} />
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <div className="p-2 border-t bg-[var(--muted)]/20">
              <Button
                variant="ghost"
                size="sm"
                onClick={addColumn}
                className="gap-2 text-[var(--primary)] hover:text-[var(--primary)]"
              >
                <Plus size={14} /> Add Column
              </Button>
            </div>
          </div>

          {showSQL && (
            <div className="bg-slate-950 p-4 rounded-md font-mono text-xs text-blue-300 whitespace-pre overflow-x-auto">
              {generateSQL() || "-- No changes"}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-4 border-t border-[var(--border)]">
            <Button variant="ghost" onClick={onCancel}>
              Cancel
            </Button>
            <Button
              variant="primary"
              onClick={handleSave}
              className="gap-2"
              loading={isLoading}
            >
              <Save size={14} /> Save Changes
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
