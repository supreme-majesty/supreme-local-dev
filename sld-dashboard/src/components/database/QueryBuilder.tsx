import { useState, useEffect } from "react";
import { useTables, useTableColumns } from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";
import { 
  Plus, 
  Trash2, 
  Database, 
  Table as TableIcon, 
  Filter, 
  ArrowDownUp, 
  Columns,
  Sigma,
  Code
} from "lucide-react";

interface QueryBuilderProps {
  database: string;
  onGenerate: (sql: string) => void;
  onClose: () => void;
}

interface TableSelection {
  id: string;
  name: string;
  alias: string;
  joinType: "FROM" | "INNER JOIN" | "LEFT JOIN" | "RIGHT JOIN";
  leftTableAlias?: string;
  leftColumn?: string;
  rightColumn?: string;
}

interface ColumnSelection {
  tableAlias: string;
  columnName: string;
  aggregate?: "COUNT" | "SUM" | "AVG" | "MIN" | "MAX";
}

interface Condition {
  tableAlias: string;
  column: string;
  operator: string;
  value: string;
}

interface OrderBy {
  tableAlias: string;
  column: string;
  direction: "ASC" | "DESC";
}

export function QueryBuilder({ database, onGenerate, onClose }: QueryBuilderProps) {
  const { data: allTables = [] } = useTables(database);
  
  const [selectedTables, setSelectedTables] = useState<TableSelection[]>([]);
  const [selectedColumns, setSelectedColumns] = useState<ColumnSelection[]>([]);
  const [conditions, setConditions] = useState<Condition[]>([]);
  const [matchType, setMatchType] = useState<"AND" | "OR">("AND");
  const [orderBy, setOrderBy] = useState<OrderBy | null>(null);
  const [limit, setLimit] = useState<string>("100");

  // Fetch columns for all selected tables
  useEffect(() => {
    const fetchColumns = async () => {
      const newColumnsByAlias: Record<string, any[]> = {};
      for (const t of selectedTables) {
        if (!newColumnsByAlias[t.alias]) {
          // This is a bit hacky since we can't call hooks in a loop
          // In a real app, we'd have a separate component for each table's column fetching
          // or a bulk API. For now, we'll assume the tables are already cached by react-query
          // since they were likely visited in the browse view.
        }
      }
    };
    fetchColumns();
  }, [selectedTables]);

  const handleAddTable = (tableName: string) => {
    const alias = `t${selectedTables.length + 1}`;
    const newTable: TableSelection = {
      id: Math.random().toString(36).substr(2, 9),
      name: tableName,
      alias,
      joinType: selectedTables.length === 0 ? "FROM" : "INNER JOIN",
      leftTableAlias: selectedTables.length > 0 ? selectedTables[selectedTables.length - 1].alias : undefined
    };
    setSelectedTables([...selectedTables, newTable]);
  };

  const handleRemoveTable = (id: string) => {
    const tableToRemove = selectedTables.find(t => t.id === id);
    if (!tableToRemove) return;

    setSelectedTables(selectedTables.filter(t => t.id !== id));
    setSelectedColumns(selectedColumns.filter(c => c.tableAlias !== tableToRemove.alias));
    setConditions(conditions.filter(c => c.tableAlias !== tableToRemove.alias));
    if (orderBy?.tableAlias === tableToRemove.alias) setOrderBy(null);
  };

  const handleToggleColumn = (tableAlias: string, col: string) => {
    const exists = selectedColumns.find(c => c.tableAlias === tableAlias && c.columnName === col && !c.aggregate);
    if (exists) {
      setSelectedColumns(selectedColumns.filter(c => !(c.tableAlias === tableAlias && c.columnName === col && !c.aggregate)));
    } else {
      setSelectedColumns([...selectedColumns, { tableAlias, columnName: col }]);
    }
  };

  const handleAddAggregate = (tableAlias: string, col: string, aggregate: ColumnSelection['aggregate']) => {
    setSelectedColumns([...selectedColumns, { tableAlias, columnName: col, aggregate }]);
  };

  const generateSQL = () => {
    if (selectedTables.length === 0) return;
    
    let sql = "SELECT ";
    
    if (selectedColumns.length === 0) {
      sql += "*";
    } else {
      sql += selectedColumns.map(c => {
        const colRef = `${c.tableAlias}.\`${c.columnName}\``;
        if (c.aggregate) {
          return `${c.aggregate}(${colRef}) AS ${c.aggregate.toLowerCase()}_${c.columnName}`;
        }
        return colRef;
      }).join(", ");
    }
    
    // FROM and JOINs
    const first = selectedTables[0];
    sql += `\nFROM \`${first.name}\` AS ${first.alias}`;
    
    for (let i = 1; i < selectedTables.length; i++) {
      const t = selectedTables[i];
      sql += `\n${t.joinType} \`${t.name}\` AS ${t.alias}`;
      if (t.leftTableAlias && t.leftColumn && t.rightColumn) {
        sql += ` ON ${t.alias}.\`${t.rightColumn}\` = ${t.leftTableAlias}.\`${t.leftColumn}\``;
      }
    }
    
    if (conditions.length > 0) {
      const validConditions = conditions.filter(c => c.operator.includes("NULL") || c.value.trim() !== "");
      if (validConditions.length > 0) {
        sql += "\nWHERE " + validConditions.map(c => {
          const colRef = `${c.tableAlias}.\`${c.column}\``;
          const trimmedValue = c.value.trim();
          const isNumeric = !isNaN(Number(trimmedValue)) && trimmedValue !== "";
          const needsQuotes = !isNumeric && !c.operator.includes("NULL");
          const val = needsQuotes ? `'${trimmedValue.replace(/'/g, "''")}'` : trimmedValue;
          
          if (c.operator.includes("NULL")) return `${colRef} ${c.operator}`;
          if (c.operator === "LIKE") return `${colRef} LIKE '%${trimmedValue.replace(/'/g, "''")}%'`;
          return `${colRef} ${c.operator} ${val}`;
        }).join(` ${matchType} `);
      }
    }

    const aggregates = selectedColumns.filter(c => c.aggregate);
    const nonAggregates = selectedColumns.filter(c => !c.aggregate);
    
    if (aggregates.length > 0 && nonAggregates.length > 0) {
      sql += "\nGROUP BY " + nonAggregates.map(c => `${c.tableAlias}.\`${c.columnName}\``).join(", ");
    }
    
    if (orderBy && orderBy.column) {
      sql += `\nORDER BY ${orderBy.tableAlias}.\`${orderBy.column}\` ${orderBy.direction}`;
    }
    
    if (limit && !isNaN(Number(limit))) {
      sql += `\nLIMIT ${limit}`;
    }
    
    sql += ";";
    onGenerate(sql);
  };

  return (
    <div className="bg-[var(--card)] border border-[var(--border)] rounded-xl p-6 shadow-xl space-y-6 mb-6 animate-in fade-in zoom-in-95 duration-200">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-bold flex items-center gap-2">
          <div className="p-1.5 rounded-lg bg-[var(--primary)] text-white">
            <Code size={18} />
          </div>
          Visual Query Builder 2.0
        </h3>
        <Button variant="ghost" size="sm" onClick={onClose} className="hover:bg-red-500/10 hover:text-red-500 transition-colors">
          <X size={18} />
        </Button>
      </div>

      <div className="space-y-6">
        {/* Tables and Joins */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <label className="text-sm font-semibold flex items-center gap-2">
              <TableIcon size={16} className="text-blue-400" /> Tables & Relationships
            </label>
            <div className="flex gap-2">
              <select 
                className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-[var(--primary)]"
                onChange={(e) => {
                  if (e.target.value) handleAddTable(e.target.value);
                  e.target.value = "";
                }}
                value=""
              >
                <option value="">+ Add Table</option>
                {allTables.map(t => <option key={t.name} value={t.name}>{t.name}</option>)}
              </select>
            </div>
          </div>

          <div className="space-y-3">
            {selectedTables.map((t, idx) => (
              <TableConfig 
                key={t.id}
                table={t}
                database={database}
                allTables={selectedTables}
                isFirst={idx === 0}
                onUpdate={(updated) => {
                  const newTables = [...selectedTables];
                  newTables[idx] = updated;
                  setSelectedTables(newTables);
                }}
                onRemove={() => handleRemoveTable(t.id)}
                onToggleColumn={(col) => handleToggleColumn(t.alias, col)}
                onAddAggregate={(col, agg) => handleAddAggregate(t.alias, col, agg)}
                selectedColumns={selectedColumns.filter(c => c.tableAlias === t.alias)}
              />
            ))}
            {selectedTables.length === 0 && (
              <div className="text-center py-8 border-2 border-dashed border-[var(--border)] rounded-xl bg-[var(--muted)]/10">
                <p className="text-[var(--muted-foreground)]">Select a starting table to begin building your query</p>
              </div>
            )}
          </div>
        </div>

        {selectedTables.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-8 pt-4 border-t border-[var(--border)]">
            {/* WHERE Conditions */}
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <label className="text-sm font-semibold flex items-center gap-2">
                  <Filter size={16} className="text-orange-400" /> Conditions (WHERE)
                </label>
                <div className="flex items-center gap-2">
                   {conditions.length > 1 && (
                    <div className="flex items-center gap-1 bg-[var(--muted)]/50 p-1 rounded-lg text-xs">
                      <button 
                        onClick={() => setMatchType("AND")}
                        className={`px-2 py-0.5 rounded ${matchType === "AND" ? "bg-[var(--primary)] text-white" : "hover:bg-[var(--muted)]"}`}
                      >
                        AND
                      </button>
                      <button 
                        onClick={() => setMatchType("OR")}
                        className={`px-2 py-0.5 rounded ${matchType === "OR" ? "bg-[var(--primary)] text-white" : "hover:bg-[var(--muted)]"}`}
                      >
                        OR
                      </button>
                    </div>
                  )}
                  <Button variant="secondary" size="sm" className="h-7 text-xs gap-1" onClick={() => setConditions([...conditions, { tableAlias: selectedTables[0].alias, column: '', operator: '=', value: '' }])}>
                    <Plus size={14} /> Add Condition
                  </Button>
                </div>
              </div>

              <div className="space-y-2">
                {conditions.map((c, i) => (
                  <div key={i} className="flex items-center gap-2 animate-in slide-in-from-left-2 duration-200">
                    <select 
                      className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-2 py-1.5 text-xs w-24"
                      value={c.tableAlias}
                      onChange={(e) => {
                        const newConditions = [...conditions];
                        newConditions[i].tableAlias = e.target.value;
                        setConditions(newConditions);
                      }}
                    >
                      {selectedTables.map(t => <option key={t.alias} value={t.alias}>{t.alias} ({t.name})</option>)}
                    </select>
                    <ColumnSelect 
                      database={database}
                      table={selectedTables.find(t => t.alias === c.tableAlias)?.name || ""}
                      value={c.column}
                      onChange={(col) => {
                        const newConditions = [...conditions];
                        newConditions[i].column = col;
                        setConditions(newConditions);
                      }}
                    />
                    <select 
                      className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-2 py-1.5 text-xs w-24"
                      value={c.operator}
                      onChange={(e) => {
                        const newConditions = [...conditions];
                        newConditions[i].operator = e.target.value;
                        setConditions(newConditions);
                      }}
                    >
                      <option value="=">=</option>
                      <option value="!=">!=</option>
                      <option value=">">&gt;</option>
                      <option value="<">&lt;</option>
                      <option value="LIKE">LIKE</option>
                      <option value="IS NULL">IS NULL</option>
                    </select>
                    {!c.operator.includes("NULL") && (
                      <input 
                        className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-2 py-1.5 text-xs flex-1 outline-none focus:ring-1 focus:ring-[var(--primary)]"
                        value={c.value}
                        onChange={(e) => {
                          const newConditions = [...conditions];
                          newConditions[i].value = e.target.value;
                          setConditions(newConditions);
                        }}
                        placeholder="Value..."
                      />
                    )}
                    <button onClick={() => setConditions(conditions.filter((_, idx) => idx !== i))} className="text-red-400 hover:text-red-500 transition-colors">
                      <Trash2 size={16} />
                    </button>
                  </div>
                ))}
              </div>
            </div>

            {/* Order & Limit */}
            <div className="space-y-6">
              <div className="space-y-3">
                <label className="text-sm font-semibold flex items-center gap-2">
                  <ArrowDownUp size={16} className="text-purple-400" /> Sorting & Limits
                </label>
                <div className="flex items-center gap-2">
                  <select 
                    className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-2 py-1.5 text-xs w-24"
                    value={orderBy?.tableAlias || ""}
                    onChange={(e) => setOrderBy(e.target.value ? { tableAlias: e.target.value, column: '', direction: 'DESC' } : null)}
                  >
                    <option value="">None</option>
                    {selectedTables.map(t => <option key={t.alias} value={t.alias}>{t.alias}</option>)}
                  </select>
                  {orderBy && (
                    <>
                      <ColumnSelect 
                        database={database}
                        table={selectedTables.find(t => t.alias === orderBy.tableAlias)?.name || ""}
                        value={orderBy.column}
                        onChange={(col) => setOrderBy({...orderBy, column: col})}
                      />
                      <select 
                        className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-2 py-1.5 text-xs w-24"
                        value={orderBy.direction}
                        onChange={(e) => setOrderBy({...orderBy, direction: e.target.value as any})}
                      >
                        <option value="ASC">ASC</option>
                        <option value="DESC">DESC</option>
                      </select>
                    </>
                  )}
                </div>
              </div>

              <div className="space-y-3">
                <label className="text-sm font-semibold">Row Limit</label>
                <input 
                  type="number"
                  className="w-full max-w-[120px] bg-[var(--background)] border border-[var(--border)] rounded-lg px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-[var(--primary)]"
                  value={limit}
                  onChange={(e) => setLimit(e.target.value)}
                />
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="pt-4 border-t border-[var(--border)] flex justify-end gap-3">
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button onClick={generateSQL} disabled={selectedTables.length === 0} className="gap-2 bg-[var(--primary)] text-white px-6">
          <Database size={16} /> Generate SQL & Apply
        </Button>
      </div>
    </div>
  );
}

function TableConfig({ 
  table, 
  database, 
  allTables,
  isFirst, 
  onUpdate, 
  onRemove,
  onToggleColumn,
  onAddAggregate,
  selectedColumns
}: { 
  table: TableSelection; 
  database: string;
  allTables: TableSelection[];
  isFirst: boolean;
  onUpdate: (t: TableSelection) => void;
  onRemove: () => void;
  onToggleColumn: (col: string) => void;
  onAddAggregate: (col: string, agg: ColumnSelection['aggregate']) => void;
  selectedColumns: ColumnSelection[];
}) {
  const { data: columns = [] } = useTableColumns(database, table.name);
  const [showColumns, setShowColumns] = useState(false);

  return (
    <div className="border border-[var(--border)] rounded-xl bg-[var(--muted)]/5 overflow-hidden">
      <div className="p-3 flex items-center justify-between bg-[var(--muted)]/20">
        <div className="flex items-center gap-3">
          <div className="flex flex-col">
            <span className="text-xs font-bold text-[var(--primary)]">{table.alias}</span>
            <span className="text-sm font-semibold">{table.name}</span>
          </div>
          
          {!isFirst && (
            <div className="flex items-center gap-2 ml-4 px-3 py-1 bg-[var(--background)] border border-[var(--border)] rounded-lg">
              <select 
                className="bg-transparent text-xs font-medium outline-none"
                value={table.joinType}
                onChange={(e) => onUpdate({...table, joinType: e.target.value as any})}
              >
                <option value="INNER JOIN">INNER JOIN</option>
                <option value="LEFT JOIN">LEFT JOIN</option>
                <option value="RIGHT JOIN">RIGHT JOIN</option>
              </select>
              <span className="text-[var(--muted-foreground)]">ON</span>
              <select 
                className="bg-transparent text-xs outline-none max-w-[80px]"
                value={table.rightColumn || ""}
                onChange={(e) => onUpdate({...table, rightColumn: e.target.value})}
              >
                <option value="">Column</option>
                {columns.map(c => <option key={c.name} value={c.name}>{c.name}</option>)}
              </select>
              <span className="text-[var(--muted-foreground)]">=</span>
              <select 
                className="bg-transparent text-xs outline-none"
                value={table.leftTableAlias || ""}
                onChange={(e) => onUpdate({...table, leftTableAlias: e.target.value})}
              >
                {allTables.filter(t => t.alias !== table.alias).map(t => (
                  <option key={t.alias} value={t.alias}>{t.alias}</option>
                ))}
              </select>
              <select 
                className="bg-transparent text-xs outline-none max-w-[80px]"
                value={table.leftColumn || ""}
                onChange={(e) => onUpdate({...table, leftColumn: e.target.value})}
              >
                <option value="">Column</option>
                {/* This is still limited as we don't have columns for the left table here easily */}
                {/* In a full version, we'd fetch these as well */}
                {columns.map(c => <option key={c.name} value={c.name}>{c.name}</option>)}
              </select>
            </div>
          )}
        </div>
        
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" className="h-7 text-xs gap-1" onClick={() => setShowColumns(!showColumns)}>
            <Columns size={14} /> {selectedColumns.length || 'All'} Columns
          </Button>
          <button onClick={onRemove} className="p-1.5 text-[var(--muted-foreground)] hover:text-red-500 transition-colors">
            <Trash2 size={16} />
          </button>
        </div>
      </div>

      {showColumns && (
        <div className="p-4 bg-[var(--background)] grid grid-cols-2 md:grid-cols-4 gap-2 animate-in slide-in-from-top-2">
          {columns.map(c => {
            const isSelected = selectedColumns.some(sc => sc.columnName === c.name && !sc.aggregate);
            return (
              <div key={c.name} className="flex items-center justify-between group p-1.5 rounded hover:bg-[var(--muted)]/30">
                <label className="flex items-center gap-2 text-xs cursor-pointer flex-1">
                  <input 
                    type="checkbox" 
                    checked={isSelected}
                    onChange={() => onToggleColumn(c.name)}
                    className="rounded border-[var(--border)] text-[var(--primary)]"
                  />
                  <span className={isSelected ? 'font-bold' : ''}>{c.name}</span>
                </label>
                <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button onClick={() => onAddAggregate(c.name, 'COUNT')} title="Count" className="p-0.5 hover:bg-[var(--primary)]/10 rounded text-[var(--primary)]"><Sigma size={10} /></button>
                  <button onClick={() => onAddAggregate(c.name, 'SUM')} title="Sum" className="p-0.5 hover:bg-green-500/10 rounded text-green-500 font-bold text-[8px]">Σ</button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function ColumnSelect({ database, table, value, onChange }: { database: string, table: string, value: string, onChange: (v: string) => void }) {
  const { data: columns = [] } = useTableColumns(database, table);
  return (
    <select 
      className="bg-[var(--background)] border border-[var(--border)] rounded-lg px-2 py-1.5 text-xs flex-1"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">Column...</option>
      {columns.map(c => <option key={c.name} value={c.name}>{c.name}</option>)}
    </select>
  );
}

const X = ({ size, className }: { size: number, className?: string }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className}><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
);
