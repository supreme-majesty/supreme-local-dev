import { useState } from "react";
import { 
  GitCompare, 
  Check, 
  Database, 
  Code,
  FileCode,
  RefreshCw,
  Plus,
  Trash2,
  Edit,
  ArrowRightLeft,
  ChevronRight
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { useSchemaDiff, useDatabases } from "@/hooks/use-database";

interface SchemaDiffToolProps {
  currentDatabase: string;
}

export function SchemaDiffTool({ currentDatabase }: SchemaDiffToolProps) {
  const { data: databases = [] } = useDatabases();
  const [targetDB, setTargetDB] = useState<string>("");
  const { data: diff, isLoading } = useSchemaDiff(currentDatabase, targetDB);
  const [showSQL, setShowSQL] = useState(false);

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <GitCompare className="text-blue-500" /> Schema Diff & Sync
          </h2>
          <p className="text-[var(--muted-foreground)]">Compare structures and generate synchronization scripts</p>
        </div>
      </div>

      <Card className="border-[var(--border)] overflow-hidden">
        <CardContent className="p-6 bg-[var(--muted)]/20">
          <div className="flex flex-col md:flex-row items-center justify-center gap-8 py-4">
            <div className="flex flex-col items-center gap-2 flex-1 max-w-[200px]">
              <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase">Source (Current)</div>
              <div className="w-full p-4 bg-[var(--background)] border border-[var(--primary)]/50 rounded-xl flex items-center gap-3 shadow-lg shadow-[var(--primary)]/5">
                <Database size={20} className="text-[var(--primary)]" />
                <span className="font-bold truncate">{currentDatabase}</span>
              </div>
            </div>

            <div className="bg-[var(--background)] p-2 rounded-full border border-[var(--border)] shadow-sm">
              <ArrowRightLeft size={20} className="text-[var(--muted-foreground)]" />
            </div>

            <div className="flex flex-col items-center gap-2 flex-1 max-w-[200px]">
              <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase">Target (Reference)</div>
              <select 
                className="w-full p-4 bg-[var(--background)] border border-[var(--border)] rounded-xl outline-none focus:ring-2 focus:ring-[var(--primary)] transition-all font-bold shadow-sm"
                value={targetDB}
                onChange={(e) => setTargetDB(e.target.value)}
              >
                <option value="">Select Database</option>
                {databases.filter(d => d.name !== currentDatabase).map(d => (
                  <option key={d.name} value={d.name}>{d.name}</option>
                ))}
              </select>
            </div>
          </div>
        </CardContent>
      </Card>

      {!targetDB ? (
        <div className="py-20 text-center border-2 border-dashed border-[var(--border)] rounded-2xl bg-[var(--muted)]/5">
          <GitCompare size={48} className="mx-auto text-[var(--muted-foreground)]/20 mb-4" />
          <h3 className="text-lg font-semibold text-[var(--muted-foreground)]">Compare your schema with another database</h3>
          <p className="text-sm text-[var(--muted-foreground)]/60">Select a target database above to see structural differences</p>
        </div>
      ) : isLoading ? (
        <div className="py-20 text-center animate-pulse">
          <RefreshCw size={48} className="mx-auto text-[var(--primary)]/20 mb-4 animate-spin" />
          <p className="text-[var(--muted-foreground)]">Analyzing schemas...</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <StatCard title="Missing Tables" count={diff?.tables_to_create?.length || 0} icon={<Plus size={16} />} color="green" />
            <StatCard title="Modified Tables" count={diff?.table_diffs?.length || 0} icon={<Edit size={16} />} color="blue" />
            <StatCard title="Tables to Drop" count={diff?.tables_to_drop?.length || 0} icon={<Trash2 size={16} />} color="red" />
          </div>

          <Card className="border-[var(--border)]">
            <CardHeader className="flex flex-row items-center justify-between border-b border-[var(--border)] py-4">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <FileCode size={16} className="text-blue-500" /> Synchronization SQL
              </CardTitle>
              <Button size="sm" variant="outline" className="gap-2" onClick={() => setShowSQL(!showSQL)}>
                <Code size={14} /> {showSQL ? 'Hide' : 'View'} SQL
              </Button>
            </CardHeader>
            <CardContent className="p-0">
              {showSQL ? (
                <div className="relative">
                   <pre className="p-6 bg-[var(--muted)]/30 text-xs font-mono overflow-auto max-h-[400px] text-blue-400">
                    {diff?.sync_sql || "-- No changes needed. Schemas are identical."}
                  </pre>
                  {diff?.sync_sql && (
                    <div className="absolute top-4 right-4 flex gap-2">
                      <Button size="sm" className="gap-2 bg-green-600 hover:bg-green-700">
                        <Check size={14} /> Run Sync
                      </Button>
                    </div>
                  )}
                </div>
              ) : (
                <div className="divide-y divide-[var(--border)]/50">
                  {diff?.table_diffs.map((t: any) => (
                    <div key={t.table_name} className="p-4 flex items-center justify-between hover:bg-[var(--muted)]/30 transition-colors">
                      <div className="flex items-center gap-4">
                        <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center text-blue-500">
                          <Database size={20} />
                        </div>
                        <div>
                          <div className="font-bold text-sm">{t.table_name}</div>
                          <div className="flex gap-2 mt-1">
                            {t.columns_to_add.length > 0 && <span className="text-[9px] bg-green-500/10 text-green-500 px-1.5 py-0.5 rounded font-bold">+{t.columns_to_add.length} COLS</span>}
                            {t.columns_to_alter.length > 0 && <span className="text-[9px] bg-blue-500/10 text-blue-500 px-1.5 py-0.5 rounded font-bold">{t.columns_to_alter.length} CHANGES</span>}
                            {t.columns_to_drop.length > 0 && <span className="text-[9px] bg-red-500/10 text-red-500 px-1.5 py-0.5 rounded font-bold">-{t.columns_to_drop.length} COLS</span>}
                          </div>
                        </div>
                      </div>
                      <ChevronRight size={16} className="text-[var(--muted-foreground)]" />
                    </div>
                  ))}
                  {diff?.tables_to_create.map((t: any) => (
                    <div key={t} className="p-4 flex items-center justify-between hover:bg-[var(--muted)]/30 transition-colors bg-green-500/5">
                      <div className="flex items-center gap-4">
                        <div className="w-10 h-10 rounded-lg bg-green-500/10 flex items-center justify-center text-green-500">
                          <Plus size={20} />
                        </div>
                        <div>
                          <div className="font-bold text-sm">{t}</div>
                          <div className="text-[10px] text-green-600 font-bold uppercase tracking-widest">New Table</div>
                        </div>
                      </div>
                    </div>
                  ))}
                  {diff?.tables_to_drop.map((t: any) => (
                    <div key={t} className="p-4 flex items-center justify-between hover:bg-[var(--muted)]/30 transition-colors bg-red-500/5">
                      <div className="flex items-center gap-4">
                        <div className="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center text-red-500">
                          <Trash2 size={20} />
                        </div>
                        <div>
                          <div className="font-bold text-sm">{t}</div>
                          <div className="text-[10px] text-red-600 font-bold uppercase tracking-widest">To be Dropped</div>
                        </div>
                      </div>
                    </div>
                  ))}
                  {(!diff || (diff.tables_to_create.length === 0 && diff.table_diffs.length === 0 && diff.tables_to_drop.length === 0)) && (
                    <div className="p-12 text-center">
                      <Check size={32} className="mx-auto text-green-500/30 mb-2" />
                      <p className="text-sm text-[var(--muted-foreground)] italic">Schemas are perfectly synchronized.</p>
                    </div>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}

function StatCard({ title, count, icon, color }: any) {
  const colors: any = {
    green: "bg-green-500/10 text-green-500",
    blue: "bg-blue-500/10 text-blue-500",
    red: "bg-red-500/10 text-red-500"
  };

  return (
    <Card className="border-[var(--border)] overflow-hidden">
      <CardContent className="p-4 flex items-center gap-4">
        <div className={`p-2 rounded-lg ${colors[color]}`}>
          {icon}
        </div>
        <div>
          <div className="text-xl font-bold font-mono">{count}</div>
          <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase tracking-wider">{title}</div>
        </div>
      </CardContent>
    </Card>
  );
}
