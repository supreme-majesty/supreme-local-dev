import { 
  History, 
  Terminal, 
  Clock,
  PlusCircle,
  MinusCircle,
  Edit3,
  Calendar,
  AlertCircle
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { useAuditLog } from "@/hooks/use-database";
import { formatDate } from "@/lib/utils";

export function AuditTimeline() {
  const { data: logs = [], isLoading } = useAuditLog();

  const getActionIcon = (action: string) => {
    switch (action.toUpperCase()) {
      case 'CREATE': return <PlusCircle className="text-green-500" size={16} />;
      case 'DROP': return <MinusCircle className="text-red-500" size={16} />;
      case 'ALTER': return <Edit3 className="text-amber-500" size={16} />;
      default: return <Terminal className="text-blue-500" size={16} />;
    }
  };

  if (isLoading) return (
    <div className="flex flex-col items-center justify-center p-20 animate-pulse">
      <History size={48} className="text-[var(--primary)]/20 mb-4 animate-spin" />
      <p className="text-[var(--muted-foreground)] italic">Loading audit timeline...</p>
    </div>
  );

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <History className="text-amber-500" /> Schema Audit Log
          </h2>
          <p className="text-[var(--muted-foreground)]">Track and verify structural changes to your databases</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 bg-amber-500/10 text-amber-500 rounded-full border border-amber-500/20 text-[10px] font-bold">
           AUTOMATIC RECORDING ACTIVE
        </div>
      </div>

      <Card className="border-[var(--border)] overflow-hidden">
        <CardContent className="p-0">
          {logs.length === 0 ? (
            <div className="p-20 text-center">
              <Terminal size={48} className="mx-auto text-[var(--muted-foreground)]/20 mb-4" />
              <h3 className="text-lg font-semibold text-[var(--muted-foreground)]">No changes recorded yet</h3>
              <p className="text-sm text-[var(--muted-foreground)]/60">Every schema change made via the console will be automatically logged here.</p>
            </div>
          ) : (
            <div className="relative">
              <div className="absolute left-8 top-0 bottom-0 w-px bg-[var(--border)]" />
              <div className="divide-y divide-[var(--border)]">
                {logs.map((log: any, i: number) => (
                  <div key={i} className="relative pl-16 pr-6 py-6 hover:bg-[var(--muted)]/20 transition-all group">
                    <div className="absolute left-6 top-8 w-4 h-4 rounded-full bg-[var(--card)] border-2 border-[var(--border)] flex items-center justify-center z-10 group-hover:border-[var(--primary)] transition-colors shadow-sm">
                      <div className="w-1.5 h-1.5 rounded-full bg-[var(--muted-foreground)] group-hover:bg-[var(--primary)]" />
                    </div>

                    <div className="flex flex-col md:flex-row md:items-start justify-between gap-4">
                      <div className="space-y-3 flex-1">
                        <div className="flex flex-wrap items-center gap-3">
                          <span className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-[var(--muted-foreground)]">
                             <Calendar size={12} /> {formatDate(log.timestamp)}
                             <Clock size={12} className="ml-2" /> {new Date(log.timestamp).toLocaleTimeString()}
                          </span>
                          <div className="h-3 w-px bg-[var(--border)]" />
                          <Badge variant="outline" className="flex items-center gap-2 py-0.5 border-blue-500/20 bg-blue-500/5 text-blue-500">
                             {getActionIcon(log.action)} {log.action}
                          </Badge>
                          <span className="text-sm font-mono font-bold text-[var(--foreground)]">{log.target}</span>
                        </div>
                        
                        <div className="p-4 bg-black/40 rounded-xl border border-white/5 font-mono text-[11px] text-blue-300 leading-relaxed overflow-x-auto shadow-inner">
                           <div className="flex items-center gap-2 text-[var(--muted-foreground)]/50 mb-2 pb-1 border-b border-white/5">
                              <Terminal size={10} /> SQL STATEMENT
                           </div>
                           {log.sql}
                        </div>
                      </div>

                      <div className="hidden md:block">
                         <div className="p-3 rounded-xl bg-green-500/5 border border-green-500/20 text-center min-w-[80px]">
                            <div className="text-[8px] font-bold text-green-500 uppercase mb-1">Status</div>
                            <div className="text-[10px] font-bold text-green-500 flex items-center justify-center gap-1">
                               <AlertCircle size={10} /> VERIFIED
                            </div>
                         </div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
