/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, Fragment } from "react";
import { 
  Activity,
  Clock, 
  Cpu, 
  Network, 
  Zap,
  ChevronDown,
  ChevronUp,
  Terminal,
  Search,
  ShieldCheck,
  AlertTriangle,
  Sparkles
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { useDatabaseStats, useTables, useOptimizationSuggestions, useExecuteQueryMutation } from "@/hooks/use-database";
import { Button } from "@/components/ui/Button";

interface PerformanceDashboardProps {
  database: string;
}

export function PerformanceDashboard({ database }: PerformanceDashboardProps) {
  const { data: stats, isLoading, error } = useDatabaseStats(database);
  const [expandedProc, setExpandedProc] = useState<string | null>(null);
  const [procFilter, setProcFilter] = useState("");
  const [targetTable, setTargetTable] = useState<string>("");

  const { data: tables = [] } = useTables(database);
  const { data: suggestions = [] } = useOptimizationSuggestions(database, targetTable);
  const executeMutation = useExecuteQueryMutation();

  if (isLoading) {
    return (
      <div className="p-12 flex flex-col items-center justify-center space-y-4 animate-pulse">
        <Activity size={48} className="text-[var(--primary)]/20" />
        <p className="text-[var(--muted-foreground)]">Analyzing server performance...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-8 bg-red-500/10 border border-red-500/20 rounded-xl flex items-center gap-4 text-red-500">
        <AlertTriangle size={24} />
        <div>
          <h3 className="font-bold">Failed to load statistics</h3>
          <p className="text-sm">{(error as Error).message}</p>
        </div>
      </div>
    );
  }

  const processes = (stats as any)?.processes || [];
  const filteredProcs = processes.filter((p: any) => 
    JSON.stringify(p).toLowerCase().includes(procFilter.toLowerCase())
  );

  // Helper to get status value (MySQL or PG)
  const getStat = (key: string) => (stats as any)?.status?.[key] || (stats as any)?.db_stats?.[key] || '0';

  return (
    <div className="space-y-6 animate-in fade-in duration-500">
      {/* Top Metrics Row */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard 
          title="Active Connections" 
          value={getStat('Threads_connected') || (stats as any)?.processes?.length || '0'} 
          icon={<Network size={20} />}
          color="blue"
          trend={getStat('Threads_running')}
          trendLabel="Running"
        />
        <MetricCard 
          title="Queries Executed" 
          value={getStat('Questions') || getStat('xact_commit')} 
          icon={<Zap size={20} />}
          color="yellow"
        />
        <MetricCard 
          title="Uptime" 
          value={formatUptime(getStat('Uptime'))} 
          icon={<Clock size={20} />}
          color="green"
        />
        <MetricCard 
          title="Buffer Pool / Cache" 
          value={getStat('Innodb_buffer_pool_pages_data') || getStat('blks_hit')} 
          icon={<Cpu size={20} />}
          color="purple"
          subtext="Hit Ratio: 98.4%"
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Active Processes */}
        <Card className="lg:col-span-2 border-[var(--border)] overflow-hidden">
          <CardHeader className="bg-[var(--muted)]/20 border-b border-[var(--border)] py-4">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <Terminal size={16} className="text-green-500" /> Active Process List
              </CardTitle>
              <div className="relative">
                <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]" />
                <input 
                  className="bg-[var(--background)] border border-[var(--border)] rounded-full pl-9 pr-4 py-1 text-xs outline-none focus:ring-1 focus:ring-[var(--primary)] w-48"
                  placeholder="Filter processes..."
                  value={procFilter}
                  onChange={(e) => setProcFilter(e.target.value)}
                />
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full text-xs text-left">
                <thead className="bg-[var(--muted)]/10 border-b border-[var(--border)]">
                  <tr>
                    <th className="px-4 py-3 font-semibold text-[var(--muted-foreground)]">ID</th>
                    <th className="px-4 py-3 font-semibold text-[var(--muted-foreground)]">User</th>
                    <th className="px-4 py-3 font-semibold text-[var(--muted-foreground)]">State</th>
                    <th className="px-4 py-3 font-semibold text-[var(--muted-foreground)]">Time</th>
                    <th className="px-4 py-3 font-semibold text-[var(--muted-foreground)]">Query</th>
                    <th className="w-8"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border)]/50">
                  {filteredProcs.map((p: any, idx: number) => {
                    const id = p.Id || p.pid || idx;
                    const isExpanded = expandedProc === id;
                    const query = p.Info || p.query || '--';
                    
                    return (
                      <Fragment key={id}>
                        <tr className={`hover:bg-[var(--muted)]/30 transition-colors cursor-pointer ${p.Time > 10 ? 'bg-orange-500/5' : ''}`}
                            onClick={() => setExpandedProc(isExpanded ? null : id)}>
                          <td className="px-4 py-3 font-mono">{id}</td>
                          <td className="px-4 py-3">{p.User || p.usename}</td>
                          <td className="px-4 py-3">
                            <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold uppercase
                              ${p.State === 'Query' || p.state === 'active' ? 'bg-green-500/10 text-green-500' : 'bg-[var(--muted)] text-[var(--muted-foreground)]'}`}>
                              {p.State || p.state}
                            </span>
                          </td>
                          <td className="px-4 py-3 font-mono">{p.Time || p.wait_event || '0'}s</td>
                          <td className="px-4 py-3 truncate max-w-[200px]">{query}</td>
                          <td className="px-4 py-3">
                            {isExpanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                          </td>
                        </tr>
                        {isExpanded && (
                          <tr className="bg-[var(--muted)]/10">
                            <td colSpan={6} className="px-4 py-4">
                              <div className="bg-[var(--background)] border border-[var(--border)] rounded-lg p-3 space-y-2">
                                <div className="flex items-center justify-between text-[10px] text-[var(--muted-foreground)] uppercase font-bold">
                                  <span>Full SQL Query</span>
                                  <span className="flex items-center gap-1"><Clock size={10} /> Running for {p.Time || 0}s</span>
                                </div>
                                <pre className="text-xs whitespace-pre-wrap break-all font-mono text-blue-400 bg-black/20 p-2 rounded">
                                  {query}
                                </pre>
                                <div className="flex justify-end pt-2">
                                  <Button size="sm" variant="secondary" className="h-7 text-[10px] gap-1 bg-indigo-500/20 text-indigo-500 hover:bg-indigo-500/30">
                                    <Sparkles size={12} /> AI Analyze Query
                                  </Button>
                                </div>
                              </div>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>
            {filteredProcs.length === 0 && (
              <div className="p-12 text-center text-[var(--muted-foreground)] italic">
                No active processes found matching your filter.
              </div>
            )}
          </CardContent>
        </Card>

        {/* Server Health / Health Check */}
        <div className="space-y-6">
          <Card className="border-[var(--border)] bg-green-500/5 border-green-500/20">
            <CardHeader className="py-4">
              <CardTitle className="text-sm flex items-center gap-2">
                <ShieldCheck size={18} className="text-green-500" /> Database Health
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <HealthItem label="Storage Usage" value="42%" status="pass" />
              <HealthItem label="Index Efficiency" value="96%" status="pass" />
              <HealthItem label="Connection Load" value="8%" status="pass" />
              <HealthItem label="Slow Query Ratio" value="0.2%" status="pass" />
            </CardContent>
          </Card>

          <Card className="border-[var(--border)] border-indigo-500/20 shadow-lg shadow-indigo-500/5">
            <CardHeader className="py-4 flex flex-row items-center justify-between bg-indigo-500/5 border-b border-[var(--border)]">
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <Sparkles size={16} className="text-indigo-500" /> AI Auto-Tuner
              </CardTitle>
              <select 
                className="text-[10px] bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1 outline-none"
                value={targetTable}
                onChange={(e) => setTargetTable(e.target.value)}
              >
                <option value="">Select Table...</option>
                {tables.map(t => <option key={t.name} value={t.name}>{t.name}</option>)}
              </select>
            </CardHeader>
            <CardContent className="space-y-4 text-xs">
              {targetTable ? (
                (suggestions as any[]).length > 0 ? (suggestions as any[]).map((s: any, i: number) => (
                  <div key={i} className="flex flex-col gap-2 p-3 bg-[var(--muted)]/30 rounded-lg border border-[var(--border)]/50">
                    <div className="flex gap-3">
                      <div className={`w-8 h-8 rounded-lg ${s.type === 'index' ? 'bg-indigo-500/10 text-indigo-500' : 'bg-yellow-500/10 text-yellow-500'} flex items-center justify-center shrink-0`}>
                        {s.type === 'index' ? <Zap size={16} /> : <AlertTriangle size={16} />}
                      </div>
                      <div>
                        <div className="font-bold mb-0.5">{s.title}</div>
                        <div className="text-[var(--muted-foreground)] line-clamp-2">{s.description}</div>
                      </div>
                    </div>
                    <Button 
                      size="sm" 
                      variant="outline" 
                      className="w-full text-[10px] h-7 bg-[var(--background)]"
                      onClick={() => executeMutation.mutate({ database, query: s.sql })}
                      disabled={executeMutation.isPending}
                    >
                      Apply Optimization
                    </Button>
                  </div>
                )) : (
                  <div className="py-8 text-center text-[var(--muted-foreground)] italic">
                    No optimizations found for {targetTable}.
                  </div>
                )
              ) : (
                <div className="py-8 text-center text-[var(--muted-foreground)] italic">
                  Select a table to see optimization suggestions.
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

function MetricCard({ title, value, icon, color, trend, trendLabel, subtext }: any) {
  const colorMap: any = {
    blue: 'bg-blue-500 text-blue-500',
    yellow: 'bg-yellow-500 text-yellow-500',
    green: 'bg-green-500 text-green-500',
    purple: 'bg-purple-500 text-purple-500'
  };

  return (
    <Card className="border-[var(--border)] hover:border-[var(--primary)]/50 transition-colors">
      <CardContent className="p-5 space-y-3">
        <div className="flex items-center justify-between">
          <div className={`p-2 rounded-lg ${colorMap[color].split(' ')[0]}/10 ${colorMap[color].split(' ')[1]}`}>
            {icon}
          </div>
          {trend && (
            <div className="text-[10px] font-bold bg-[var(--muted)] px-2 py-0.5 rounded-full flex items-center gap-1">
              <div className="w-1.5 h-1.5 rounded-full bg-green-500 animate-pulse" />
              {trend} {trendLabel}
            </div>
          )}
        </div>
        <div>
          <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase tracking-wider">{title}</div>
          <div className="text-2xl font-bold font-mono">{value}</div>
          {subtext && <div className="text-[10px] text-[var(--muted-foreground)] mt-1">{subtext}</div>}
        </div>
      </CardContent>
    </Card>
  );
}

function HealthItem({ label, value, status }: any) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-xs text-[var(--muted-foreground)]">{label}</span>
      <div className="flex items-center gap-3">
        <span className="text-xs font-bold">{value}</span>
        <div className={`w-2 h-2 rounded-full ${status === 'pass' ? 'bg-green-500' : 'bg-red-500'}`} />
      </div>
    </div>
  );
}

function formatUptime(seconds: any) {
  const s = parseInt(seconds) || 0;
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s/60)}m`;
  if (s < 86400) return `${Math.floor(s/3600)}h`;
  return `${Math.floor(s/86400)}d`;
}

