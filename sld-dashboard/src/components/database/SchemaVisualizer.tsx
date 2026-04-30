/* eslint-disable @typescript-eslint/no-explicit-any */
import { useTableRelationships, useTables } from "@/hooks/use-database";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Table as TableIcon, ArrowRight } from "lucide-react";

interface SchemaVisualizerProps {
  database: string;
}

export function SchemaVisualizer({ database }: SchemaVisualizerProps) {
  const { data: relationships = [] } = useTableRelationships(database) as { data: any[] };
  const { data: tables = [] } = useTables(database) as { data: any[] };

  if (tables.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 border border-dashed border-[var(--border)] rounded-lg text-[var(--muted-foreground)]">
        No tables found in this database.
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {tables.map((table) => {
          const tableRels = (relationships as any[]).filter(
            (r: any) => r.from_table === table.name || r.to_table === table.name
          );

          return (
            <Card key={table.name} className="bg-[var(--card)] hover:shadow-md transition-all border-[var(--border)] group">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-mono flex items-center gap-2 text-[var(--primary)]">
                  <TableIcon size={16} />
                  {table.name}
                  <span className="ml-auto text-[10px] font-normal text-[var(--muted-foreground)] px-2 py-0.5 bg-[var(--muted)] rounded-full">
                    {table.row_count} rows
                  </span>
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-2">
                  {tableRels.length > 0 ? (
                    <div className="text-[10px] uppercase tracking-wider text-[var(--muted-foreground)] font-semibold mb-1">
                      Relationships
                    </div>
                  ) : (
                    <div className="text-[10px] text-[var(--muted-foreground)] italic">
                      No relationships found
                    </div>
                  )}
                  {(tableRels as any[]).map((rel: any, i: number) => {
                    const isFrom = rel.from_table === table.name;
                    return (
                      <div key={i} className="flex items-center gap-2 text-xs py-1 px-2 rounded bg-[var(--muted)]/50 border border-[var(--border)]/50 group-hover:border-[var(--primary)]/20 transition-colors">
                        <span className="font-medium truncate max-w-[80px]">{isFrom ? rel.from_column : rel.to_column}</span>
                        <ArrowRight size={10} className="text-[var(--muted-foreground)]" />
                        <span className="text-[var(--primary)] font-semibold truncate max-w-[80px]">
                          {isFrom ? rel.to_table : rel.from_table}
                        </span>
                        <span className="text-[var(--muted-foreground)] opacity-50">({isFrom ? rel.to_column : rel.from_column})</span>
                      </div>
                    );
                  })}
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>

      {/* Stats Summary */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="p-4 rounded-xl bg-[var(--primary)]/5 border border-[var(--primary)]/10">
          <div className="text-[var(--muted-foreground)] text-xs uppercase font-semibold">Total Tables</div>
          <div className="text-2xl font-bold">{tables.length}</div>
        </div>
        <div className="p-4 rounded-xl bg-green-500/5 border border-green-500/10">
          <div className="text-[var(--muted-foreground)] text-xs uppercase font-semibold">Total Relationships</div>
          <div className="text-2xl font-bold">{relationships.length}</div>
        </div>
        <div className="p-4 rounded-xl bg-orange-500/5 border border-orange-500/10">
          <div className="text-[var(--muted-foreground)] text-xs uppercase font-semibold">DB Engine</div>
          <div className="text-2xl font-bold">{tables[0]?.engine || "MySQL"}</div>
        </div>
      </div>
    </div>
  );
}
