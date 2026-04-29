import { useState, useEffect } from "react";
import { useTables, useTableColumns } from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";
import { Plus, Trash2, Database, Table as TableIcon, Filter, ArrowDownUp, Columns } from "lucide-react";

interface QueryBuilderProps {
  database: string;
  onGenerate: (sql: string) => void;
  onClose: () => void;
}

interface Condition {
  column: string;
  operator: string;
  value: string;
}

interface OrderBy {
  column: string;
  direction: "ASC" | "DESC";
}

export function QueryBuilder({ database, onGenerate, onClose }: QueryBuilderProps) {
  const { data: tables = [] } = useTables(database);
  
  const [selectedTable, setSelectedTable] = useState<string>("");
  const { data: columns = [] } = useTableColumns(database, selectedTable);
  
  const [selectedColumns, setSelectedColumns] = useState<string[]>([]);
  const [conditions, setConditions] = useState<Condition[]>([]);
  const [matchType, setMatchType] = useState<"AND" | "OR">("AND");
  const [orderBy, setOrderBy] = useState<OrderBy | null>(null);
  const [limit, setLimit] = useState<string>("100");

  // When table changes, reset selections but default to all columns
  useEffect(() => {
    if (selectedTable && columns.length > 0) {
      setSelectedColumns(columns.map(c => c.name));
      setConditions([]);
      setOrderBy(null);
    }
  }, [selectedTable, columns]);

  const handleToggleColumn = (col: string) => {
    setSelectedColumns(prev => 
      prev.includes(col) ? prev.filter(c => c !== col) : [...prev, col]
    );
  };

  const handleAddCondition = () => {
    if (columns.length > 0) {
      setConditions([...conditions, { column: columns[0].name, operator: "=", value: "" }]);
    }
  };

  const handleUpdateCondition = (index: number, field: keyof Condition, value: string) => {
    const newConditions = [...conditions];
    newConditions[index] = { ...newConditions[index], [field]: value };
    setConditions(newConditions);
  };

  const handleRemoveCondition = (index: number) => {
    setConditions(conditions.filter((_, i) => i !== index));
  };

  const generateSQL = () => {
    if (!selectedTable) return;
    
    let sql = "SELECT ";
    
    if (selectedColumns.length === 0) {
      sql += "*";
    } else if (selectedColumns.length === columns.length) {
      sql += "*";
    } else {
      sql += selectedColumns.map(c => `\`${c}\``).join(", ");
    }
    
    sql += `\nFROM \`${selectedTable}\``;
    
    if (conditions.length > 0) {
      const validConditions = conditions.filter(c => c.operator.includes("NULL") || c.value.trim() !== "");
      if (validConditions.length > 0) {
        sql += "\nWHERE " + validConditions.map(c => {
          // Add quotes if value doesn't look like a number and operator isn't IS NULL
          const trimmedValue = c.value.trim();
          const isNumeric = !isNaN(Number(trimmedValue)) && trimmedValue !== "";
          const needsQuotes = !isNumeric && !c.operator.includes("NULL");
          const val = needsQuotes ? `'${trimmedValue.replace(/'/g, "''")}'` : trimmedValue;
          
          if (c.operator.includes("NULL")) {
            return `\`${c.column}\` ${c.operator}`;
          }
          if (c.operator === "LIKE") {
            return `\`${c.column}\` LIKE '%${trimmedValue.replace(/'/g, "''")}%'`;
          }
          return `\`${c.column}\` ${c.operator} ${val}`;
        }).join(` ${matchType} `);
      }
    }
    
    if (orderBy && orderBy.column) {
      sql += `\nORDER BY \`${orderBy.column}\` ${orderBy.direction}`;
    }
    
    if (limit && !isNaN(Number(limit))) {
      sql += `\nLIMIT ${limit}`;
    }
    
    sql += ";";
    onGenerate(sql);
  };

  return (
    <div className="bg-[var(--card)] border border-[var(--border)] rounded-lg p-4 shadow-sm space-y-4 mb-4">
      <div className="flex items-center justify-between">
        <h3 className="font-bold flex items-center gap-2">
          <Database size={16} className="text-[var(--primary)]" /> Visual Query Builder
        </h3>
        <Button variant="ghost" size="sm" onClick={onClose} className="h-6 px-2 text-[var(--muted-foreground)]">Close</Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        {/* Table Selection */}
        <div className="space-y-2">
          <label className="text-xs font-medium text-[var(--muted-foreground)] flex items-center gap-1">
            <TableIcon size={12} /> Table
          </label>
          <select 
            className="w-full bg-[var(--background)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
            value={selectedTable}
            onChange={(e) => setSelectedTable(e.target.value)}
          >
            <option value="">Select a table...</option>
            {tables.map(t => (
              <option key={t.name} value={t.name}>{t.name}</option>
            ))}
          </select>
        </div>

        {/* Columns */}
        <div className="md:col-span-3 space-y-2">
          <label className="text-xs font-medium text-[var(--muted-foreground)] flex items-center gap-1">
            <Columns size={12} /> Columns to Select
          </label>
          {!selectedTable ? (
            <div className="text-sm text-[var(--muted-foreground)] italic p-1.5">Select a table first</div>
          ) : (
            <div className="flex flex-wrap gap-2 max-h-24 overflow-y-auto p-2 bg-[var(--background)] border border-[var(--border)] rounded">
              {columns.map(c => (
                <label key={c.name} className="flex items-center gap-1.5 text-sm cursor-pointer bg-[var(--muted)]/50 px-2 py-1 rounded hover:bg-[var(--muted)] transition-colors">
                  <input 
                    type="checkbox" 
                    checked={selectedColumns.includes(c.name)}
                    onChange={() => handleToggleColumn(c.name)}
                    className="rounded border-[var(--border)]"
                  />
                  {c.name}
                </label>
              ))}
            </div>
          )}
        </div>
      </div>

      {selectedTable && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-2 border-t border-[var(--border)]">
          {/* WHERE Conditions */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <label className="text-xs font-medium text-[var(--muted-foreground)] flex items-center gap-1">
                  <Filter size={12} /> Conditions (WHERE)
                </label>
                {conditions.length > 1 && (
                  <div className="flex items-center gap-1 bg-[var(--muted)]/50 p-0.5 rounded text-[10px]">
                    <button 
                      onClick={() => setMatchType("AND")}
                      className={`px-1.5 py-0.5 rounded ${matchType === "AND" ? "bg-[var(--primary)] text-white" : "hover:bg-[var(--muted)]"}`}
                    >
                      ALL
                    </button>
                    <button 
                      onClick={() => setMatchType("OR")}
                      className={`px-1.5 py-0.5 rounded ${matchType === "OR" ? "bg-[var(--primary)] text-white" : "hover:bg-[var(--muted)]"}`}
                    >
                      ANY
                    </button>
                  </div>
                )}
              </div>
              <Button variant="secondary" size="sm" className="h-6 px-2 text-xs gap-1" onClick={handleAddCondition}>
                <Plus size={12} /> Add
              </Button>
            </div>
            
            {conditions.length === 0 ? (
              <div className="text-xs text-[var(--muted-foreground)] italic">No conditions added.</div>
            ) : (
              <div className="space-y-2">
                {conditions.map((c, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <select 
                      className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-xs flex-1"
                      value={c.column}
                      onChange={(e) => handleUpdateCondition(i, "column", e.target.value)}
                    >
                      {columns.map(col => <option key={col.name} value={col.name}>{col.name}</option>)}
                    </select>
                    
                    <select 
                      className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-xs w-20"
                      value={c.operator}
                      onChange={(e) => handleUpdateCondition(i, "operator", e.target.value)}
                    >
                      <option value="=">=</option>
                      <option value="!=">!=</option>
                      <option value=">">&gt;</option>
                      <option value="<">&lt;</option>
                      <option value=">=">&gt;=</option>
                      <option value="<=">&lt;=</option>
                      <option value="LIKE">LIKE</option>
                      <option value="IS NULL">IS NULL</option>
                      <option value="IS NOT NULL">IS NOT NULL</option>
                    </select>
                    
                    {!c.operator.includes("NULL") && (
                      <input 
                        type="text" 
                        placeholder="Value..."
                        className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 text-xs flex-1"
                        value={c.value}
                        onChange={(e) => handleUpdateCondition(i, "value", e.target.value)}
                      />
                    )}
                    
                    <button className="text-red-400 hover:text-red-500 p-1" onClick={() => handleRemoveCondition(i)}>
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* ORDER & LIMIT */}
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-[var(--muted-foreground)] flex items-center gap-1">
                <ArrowDownUp size={12} /> Sort (ORDER BY)
              </label>
              <div className="flex items-center gap-2">
                <select 
                  className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1.5 text-sm flex-1"
                  value={orderBy?.column || ""}
                  onChange={(e) => setOrderBy(e.target.value ? { column: e.target.value, direction: orderBy?.direction || "DESC" } : null)}
                >
                  <option value="">None</option>
                  {columns.map(c => <option key={c.name} value={c.name}>{c.name}</option>)}
                </select>
                
                {orderBy && (
                  <select 
                    className="bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1.5 text-sm w-24"
                    value={orderBy.direction}
                    onChange={(e) => setOrderBy({ ...orderBy, direction: e.target.value as "ASC" | "DESC" })}
                  >
                    <option value="ASC">ASC</option>
                    <option value="DESC">DESC</option>
                  </select>
                )}
              </div>
            </div>

            <div className="space-y-2">
              <label className="text-xs font-medium text-[var(--muted-foreground)]">
                Row Limit
              </label>
              <input 
                type="number" 
                className="w-full bg-[var(--background)] border border-[var(--border)] rounded px-3 py-1.5 text-sm"
                value={limit}
                onChange={(e) => setLimit(e.target.value)}
                min="1"
              />
            </div>
          </div>
        </div>
      )}

      <div className="pt-2 border-t border-[var(--border)] flex justify-end">
        <Button onClick={generateSQL} disabled={!selectedTable} className="gap-2">
          <Database size={14} /> Apply to Editor
        </Button>
      </div>
    </div>
  );
}
