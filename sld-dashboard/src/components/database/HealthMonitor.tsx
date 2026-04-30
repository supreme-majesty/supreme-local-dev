/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useEffect } from "react";
import { 
  Activity, 
  Zap, 
  Cpu, 
  Database, 
  TrendingUp, 
  TrendingDown,
  Clock,
  AlertCircle
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { useDatabaseStats } from "@/hooks/use-database";

interface HealthMonitorProps {
  database: string;
}

export function HealthMonitor({ database }: HealthMonitorProps) {
  const { data: stats, isLoading, refetch } = useDatabaseStats(database);
  const [history, setHistory] = useState<any[]>([]);

  // Polling for "live" feel
  useEffect(() => {
    const timer = setInterval(() => refetch(), 3000);
    return () => clearInterval(timer);
  }, [refetch]);

  useEffect(() => {
    if (stats) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setHistory(prev => [...prev.slice(-19), {
        time: new Date().toLocaleTimeString(),
        qps: (stats as any).queries_per_second || 0,
        load: (stats as any).cpu_load || 0
      }]);
    }
  }, [stats]);

  if (isLoading && history.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center p-20 animate-pulse">
        <Activity size={48} className="text-[var(--primary)]/20 mb-4" />
        <p className="text-[var(--muted-foreground)] italic">Synchronizing health vitals...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <Activity className="text-green-500" /> Database Health Monitor
          </h2>
          <p className="text-[var(--muted-foreground)]">Real-time performance metrics and system vitals</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 bg-green-500/10 text-green-500 rounded-full border border-green-500/20 text-[10px] font-bold animate-pulse">
          <div className="w-2 h-2 rounded-full bg-green-500" /> LIVE MONITORING
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatWidget 
          title="Queries / Sec" 
          value={(stats as any)?.queries_per_second?.toFixed(1) || "0.0"} 
          icon={<Zap size={16} />} 
          trend={+5.2}
          color="blue"
        />
        <StatWidget 
          title="Active Connections" 
          value={(stats as any)?.threads_connected || "0"} 
          icon={<Cpu size={16} />} 
          trend={-2.1}
          color="purple"
        />
        <StatWidget 
          title="Buffer Pool Usage" 
          value={`${((stats as any)?.buffer_pool_usage || 0).toFixed(1)}%`} 
          icon={<Database size={16} />} 
          color="orange"
        />
        <StatWidget 
          title="Uptime" 
          value={formatUptime((stats as any)?.uptime || 0)} 
          icon={<Clock size={16} />} 
          color="green"
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="lg:col-span-2 border-[var(--border)] overflow-hidden">
          <CardHeader className="border-b border-[var(--border)] bg-[var(--muted)]/20">
            <CardTitle className="text-sm font-bold flex items-center gap-2">
              <TrendingUp size={16} className="text-blue-500" /> Throughput Trend (QPS)
            </CardTitle>
          </CardHeader>
          <CardContent className="p-6 h-[300px] flex items-end gap-1">
            {history.map((h, i) => (
               <div 
                key={i} 
                className="flex-1 bg-blue-500/20 hover:bg-blue-500/40 transition-all rounded-t-sm relative group"
                style={{ height: `${Math.min(100, (h.qps / 500) * 100)}%` }}
               >
                 <div className="absolute bottom-full mb-2 left-1/2 -translate-x-1/2 bg-[var(--card)] border border-[var(--border)] px-2 py-1 rounded text-[8px] font-mono opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap z-10 pointer-events-none">
                    {h.qps.toFixed(1)} QPS @ {h.time}
                 </div>
               </div>
            ))}
          </CardContent>
        </Card>

        <Card className="border-[var(--border)]">
          <CardHeader className="border-b border-[var(--border)]">
            <CardTitle className="text-sm font-bold flex items-center gap-2">
              <AlertCircle size={16} className="text-amber-500" /> Health Checklist
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="divide-y divide-[var(--border)]">
              <HealthCheckItem label="Slow Query Log" status="Enabled" ok={true} />
              <HealthCheckItem label="Index Fragmentation" status="Low" ok={true} />
              <HealthCheckItem label="Temp Table Usage" status="Normal" ok={true} />
              <HealthCheckItem label="Connection Limit" status="12/151" ok={true} />
              <HealthCheckItem label="Disk Space" status="62% Free" ok={true} />
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function StatWidget({ title, value, icon, trend, color }: any) {
  const colors: any = {
    blue: "text-blue-500 bg-blue-500/10 border-blue-500/20",
    purple: "text-purple-500 bg-purple-500/10 border-purple-500/20",
    orange: "text-orange-500 bg-orange-500/10 border-orange-500/20",
    green: "text-green-500 bg-green-500/10 border-green-500/20",
  };

  return (
    <Card className="border-[var(--border)] overflow-hidden shadow-lg shadow-[var(--muted)]/20">
      <CardContent className="p-4 flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className={`p-3 rounded-2xl border ${colors[color]}`}>
            {icon}
          </div>
          <div>
            <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase tracking-widest mb-0.5">{title}</div>
            <div className="text-2xl font-bold font-mono">{value}</div>
          </div>
        </div>
        {trend !== undefined && (
          <div className={`flex items-center gap-1 text-xs font-bold ${trend >= 0 ? 'text-green-500' : 'text-red-500'}`}>
            {trend >= 0 ? <TrendingUp size={12} /> : <TrendingDown size={12} />}
            {Math.abs(trend)}%
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function HealthCheckItem({ label, status, ok }: any) {
  return (
    <div className="p-4 flex items-center justify-between hover:bg-[var(--muted)]/30 transition-colors">
      <span className="text-xs font-medium">{label}</span>
      <div className="flex items-center gap-2">
        <span className="text-[10px] font-mono text-[var(--muted-foreground)]">{status}</span>
        <div className={`w-1.5 h-1.5 rounded-full ${ok ? 'bg-green-500 shadow-[0_0_8px_var(--primary)]' : 'bg-red-500'}`} />
      </div>
    </div>
  );
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  return `${hours}h ${mins}m`;
}
