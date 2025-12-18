import { useState } from "react";
import { Plus, Trash2, Save, Code2 } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  CardDescription,
} from "@/components/ui/Card";

interface ColumnDef {
  id: string;
  name: string;
  type: string;
  length: string;
  default: string;
  nullable: boolean;
  primary: boolean;
  autoIncrement: boolean;
  unique: boolean;
}

interface TableCreatorProps {
  database: string;
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

export function TableCreator({
  database,
  onCancel,
  onSave,
  isLoading,
}: TableCreatorProps) {
  const [tableName, setTableName] = useState("");
  const [columns, setColumns] = useState<ColumnDef[]>([
    {
      id: crypto.randomUUID(),
      name: "id",
      type: "INT",
      length: "",
      default: "",
      nullable: false,
      primary: true,
      autoIncrement: true,
      unique: false,
    },
  ]);
  const [showSQL, setShowSQL] = useState(false);

  const addColumn = () => {
    setColumns([
      ...columns,
      {
        id: crypto.randomUUID(),
        name: "",
        type: "VARCHAR",
        length: "255",
        default: "",
        nullable: false,
        primary: false,
        autoIncrement: false,
        unique: false,
      },
    ]);
  };

  const removeColumn = (id: string) => {
    if (columns.length > 1) {
      setColumns(columns.filter((c) => c.id !== id));
    }
  };

  const updateColumn = (id: string, field: keyof ColumnDef, value: any) => {
    setColumns(
      columns.map((c) => {
        if (c.id !== id) return c;
        // Handle auto-increment logic
        if (field === "autoIncrement" && value === true) {
          // If auto-increment is on, type usually should be numeric and it should be a key
          return { ...c, [field]: value, primary: true, type: "INT" }; // Simplified assumption
        }
        return { ...c, [field]: value };
      })
    );
  };

  const generateSQL = () => {
    if (!tableName.trim()) return "";

    const lines = [`CREATE TABLE \`${tableName}\` (`];
    const primaryKeys: string[] = [];
    const uniqueKeys: string[] = [];

    const colDefs = columns
      .map((col) => {
        if (!col.name.trim()) return null;

        let def = `  \`${col.name}\` ${col.type}`;

        if (
          col.length &&
          ![
            "DATE",
            "DATETIME",
            "TIMESTAMP",
            "TEXT",
            "JSON",
            "BLOB",
            "LONGTEXT",
            "BOOLEAN",
          ].includes(col.type)
        ) {
          def += `(${col.length})`;
        }

        if (!col.nullable) {
          def += " NOT NULL";
        } else {
          def += " NULL";
        }

        if (col.autoIncrement) {
          def += " AUTO_INCREMENT";
        } else if (col.default) {
          // Handle default value quoting roughly
          if (
            col.default.toUpperCase() === "CURRENT_TIMESTAMP" ||
            col.default.toUpperCase() === "NULL" ||
            !isNaN(Number(col.default))
          ) {
            def += ` DEFAULT ${col.default}`;
          } else {
            def += ` DEFAULT '${col.default}'`;
          }
        }

        if (col.primary) primaryKeys.push(col.name);
        if (col.unique) uniqueKeys.push(col.name);

        return def;
      })
      .filter(Boolean);

    if (colDefs.length === 0) return "";

    if (primaryKeys.length > 0) {
      colDefs.push(`  PRIMARY KEY (\`${primaryKeys.join("`, `")}\`)`);
    }

    uniqueKeys.forEach((key) => {
      // Avoid duplicate key definition if it's already PK
      if (!primaryKeys.includes(key)) {
        colDefs.push(`  UNIQUE KEY \`${key}\` (\`${key}\`)`);
      }
    });

    lines.push(colDefs.join(",\n"));
    lines.push(
      `) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`
    );

    return lines.join("\n");
  };

  const handleCreate = () => {
    const sql = generateSQL();
    if (sql) {
      onSave(sql);
    }
  };

  return (
    <div className="max-w-6xl mx-auto space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Create New Table</CardTitle>
          <CardDescription>
            Define structure for a new table in database{" "}
            <strong>{database}</strong>.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex gap-4 items-end">
            <div className="flex-1 space-y-2">
              <label className="text-sm font-medium">Table Name</label>
              <Input
                placeholder="e.g. users"
                value={tableName}
                onChange={(e) => setTableName(e.target.value)}
              />
            </div>
            <div className="flex gap-2">
              <Button
                variant="ghost"
                onClick={() => setShowSQL(!showSQL)}
                className="gap-2"
              >
                <Code2 size={16} /> {showSQL ? "Hide SQL" : "Preview SQL"}
              </Button>
            </div>
          </div>

          <div className="border rounded-md overflow-hidden bg-[var(--card)]">
            <div className="overflow-x-auto">
              <table className="w-full text-sm text-left">
                <thead className="bg-[var(--muted)]/50 border-b text-xs uppercase text-[var(--muted-foreground)]">
                  <tr>
                    <th className="px-4 py-3 font-medium w-[200px]">
                      Column Name
                    </th>
                    <th className="px-4 py-3 font-medium w-[150px]">Type</th>
                    <th className="px-4 py-3 font-medium w-[100px]">Length</th>
                    <th className="px-4 py-3 font-medium w-[150px]">Default</th>
                    <th className="px-4 py-3 font-medium text-center w-[60px]">
                      Null
                    </th>
                    <th className="px-4 py-3 font-medium text-center w-[60px]">
                      Pri
                    </th>
                    <th className="px-4 py-3 font-medium text-center w-[60px]">
                      A_I
                    </th>
                    <th className="px-4 py-3 font-medium text-center w-[60px]">
                      Uni
                    </th>
                    <th className="px-4 py-3 font-medium w-[50px]"></th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {columns.map((col) => (
                    <tr
                      key={col.id}
                      className="group hover:bg-[var(--muted)]/20"
                    >
                      <td className="p-2">
                        <Input
                          value={col.name}
                          onChange={(e) =>
                            updateColumn(col.id, "name", e.target.value)
                          }
                          placeholder="Column name"
                          className="h-8"
                        />
                      </td>
                      <td className="p-2">
                        <select
                          className="w-full h-8 rounded-md border border-[var(--input)] bg-[var(--background)] px-2 text-xs"
                          value={col.type}
                          onChange={(e) =>
                            updateColumn(col.id, "type", e.target.value)
                          }
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
                          value={col.length}
                          onChange={(e) =>
                            updateColumn(col.id, "length", e.target.value)
                          }
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
                          ].includes(col.type)}
                        />
                      </td>
                      <td className="p-2">
                        <Input
                          value={col.default}
                          onChange={(e) =>
                            updateColumn(col.id, "default", e.target.value)
                          }
                          placeholder="NULL"
                          className="h-8"
                        />
                      </td>
                      <td className="p-2 text-center">
                        <input
                          type="checkbox"
                          checked={col.nullable}
                          onChange={(e) =>
                            updateColumn(col.id, "nullable", e.target.checked)
                          }
                          className="rounded border-[var(--input)]"
                        />
                      </td>
                      <td className="p-2 text-center">
                        <input
                          type="checkbox"
                          checked={col.primary}
                          onChange={(e) =>
                            updateColumn(col.id, "primary", e.target.checked)
                          }
                          className="rounded border-[var(--input)]"
                        />
                      </td>
                      <td className="p-2 text-center">
                        <input
                          type="checkbox"
                          checked={col.autoIncrement}
                          onChange={(e) =>
                            updateColumn(
                              col.id,
                              "autoIncrement",
                              e.target.checked
                            )
                          }
                          className="rounded border-[var(--input)]"
                          disabled={
                            !["INT", "BIGINT", "TINYINT"].some((t) =>
                              col.type.includes(t)
                            )
                          }
                        />
                      </td>
                      <td className="p-2 text-center">
                        <input
                          type="checkbox"
                          checked={col.unique}
                          onChange={(e) =>
                            updateColumn(col.id, "unique", e.target.checked)
                          }
                          className="rounded border-[var(--input)]"
                        />
                      </td>
                      <td className="p-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-8 w-8 p-0 text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                          onClick={() => removeColumn(col.id)}
                          disabled={columns.length === 1}
                        >
                          <Trash2 size={14} />
                        </Button>
                      </td>
                    </tr>
                  ))}
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
              {generateSQL() || "-- Complete form to generate SQL"}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-4 border-t">
            <Button variant="ghost" onClick={onCancel}>
              Cancel
            </Button>
            <Button
              variant="primary"
              onClick={handleCreate}
              className="gap-2"
              disabled={!tableName || columns.some((c) => !c.name)}
              loading={isLoading}
            >
              <Save size={14} /> Create Table
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
