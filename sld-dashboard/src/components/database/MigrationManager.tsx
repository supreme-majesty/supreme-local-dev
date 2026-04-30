/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState } from "react";
import { 
  History, 
  Plus, 
  Play, 
  RotateCcw, 
  FileText, 
  CheckCircle2, 
  Clock, 
  AlertCircle,
  Loader2
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { 
  useMigrations, 
  useInitMigrationsMutation, 
  useCreateMigrationMutation, 
  useRunMigrationMutation 
} from "@/hooks/use-database";
import { formatDate } from "@/lib/utils";

interface MigrationManagerProps {
  database: string;
}

export function MigrationManager({ database }: MigrationManagerProps) {
  const { data: migrations, isLoading, refetch } = useMigrations(database);
  const initMutation = useInitMigrationsMutation();
  const createMutation = useCreateMigrationMutation();
  const runMutation = useRunMigrationMutation();
  
  const [newMigrationName, setNewMigrationName] = useState("");
  const [isCreating, setIsCreating] = useState(false);

  if (isLoading) return (
    <div className="flex flex-col items-center justify-center p-20 animate-pulse">
      <History size={48} className="text-[var(--primary)]/20 mb-4" />
      <p className="text-[var(--muted-foreground)]">Loading migration history...</p>
    </div>
  );

  // If _sld_migrations doesn't exist (assuming backend error or empty check)
  const isInitialized = migrations !== undefined;

  const handleInit = async () => {
    await initMutation.mutateAsync({ database });
    refetch();
  };

  const handleCreate = async () => {
    if (!newMigrationName) return;
    await createMutation.mutateAsync({ database, name: newMigrationName });
    setNewMigrationName("");
    setIsCreating(false);
  };

  const handleRun = async (filename: string) => {
    await runMutation.mutateAsync({ database, filename });
  };

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <History className="text-indigo-500" /> Migration Manager
          </h2>
          <p className="text-[var(--muted-foreground)]">Track and apply schema changes for <span className="font-mono text-[var(--foreground)]">{database}</span></p>
        </div>
        {!isInitialized ? (
          <Button onClick={handleInit} disabled={initMutation.isPending} className="gap-2">
            {initMutation.isPending ? <Loader2 size={16} className="animate-spin" /> : <Play size={16} />}
            Initialize Migrations
          </Button>
        ) : (
          <Button onClick={() => setIsCreating(true)} className="gap-2 bg-indigo-600 hover:bg-indigo-700">
            <Plus size={16} /> New Migration
          </Button>
        )}
      </div>

      {!isInitialized && (
        <Card className="bg-amber-500/5 border-amber-500/20">
          <CardContent className="p-6 flex items-start gap-4">
            <AlertCircle className="text-amber-500 shrink-0" size={24} />
            <div>
              <h3 className="font-bold text-amber-500 mb-1">Migration Tracking Not Initialized</h3>
              <p className="text-sm text-[var(--muted-foreground)]">
                SLD uses a special table <code className="bg-[var(--muted)] px-1 rounded">_sld_migrations</code> to track which schema changes have been applied. Click initialize to create this table and start tracking.
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {isCreating && (
        <Card className="border-indigo-500/30 bg-indigo-500/5">
          <CardContent className="p-4 flex items-center gap-4">
            <div className="flex-1">
              <input 
                autoFocus
                placeholder="migration_name (e.g. add_users_table)"
                className="w-full bg-[var(--background)] border border-[var(--border)] rounded px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-indigo-500"
                value={newMigrationName}
                onChange={(e) => setNewMigrationName(e.target.value.replace(/\s+/g, '_').toLowerCase())}
                onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
              />
            </div>
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={() => setIsCreating(false)}>Cancel</Button>
              <Button size="sm" onClick={handleCreate} disabled={!newMigrationName || createMutation.isPending}>
                {createMutation.isPending ? <Loader2 size={14} className="animate-spin" /> : 'Create'}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Pending Migrations */}
        <div className="space-y-4">
          <h3 className="text-sm font-bold uppercase tracking-wider text-[var(--muted-foreground)] flex items-center gap-2">
            <Clock size={14} /> Pending Changes
          </h3>
          <div className="space-y-3">
            {migrations?.pending.map((m: any) => (
              <div key={m.version} className="group p-4 bg-[var(--card)] border border-[var(--border)] rounded-xl hover:border-indigo-500/50 transition-all flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-indigo-500/10 text-indigo-500">
                    <FileText size={18} />
                  </div>
                  <div>
                    <div className="font-bold text-sm">{m.name}</div>
                    <div className="text-[10px] text-[var(--muted-foreground)] font-mono">{m.version}</div>
                  </div>
                </div>
                <Button 
                  size="sm" 
                  className="gap-2" 
                  onClick={() => handleRun(`${m.version}_${m.name}.sql`)}
                  disabled={runMutation.isPending}
                >
                  <Play size={14} /> Run
                </Button>
              </div>
            ))}
            {migrations?.pending.length === 0 && (
              <div className="py-12 text-center border-2 border-dashed border-[var(--border)] rounded-xl">
                <CheckCircle2 size={32} className="mx-auto text-green-500/20 mb-2" />
                <p className="text-sm text-[var(--muted-foreground)] italic">All caught up! No pending migrations.</p>
              </div>
            )}
          </div>
        </div>

        {/* Applied History */}
        <div className="space-y-4">
          <h3 className="text-sm font-bold uppercase tracking-wider text-[var(--muted-foreground)] flex items-center gap-2">
            <RotateCcw size={14} /> Migration History
          </h3>
          <div className="space-y-3">
            {migrations?.applied.map((m: any) => (
              <div key={m.version} className="p-4 bg-[var(--muted)]/20 border border-[var(--border)]/50 rounded-xl flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-green-500/10 text-green-500">
                    <CheckCircle2 size={18} />
                  </div>
                  <div>
                    <div className="font-bold text-sm">{m.name}</div>
                    <div className="text-[10px] text-[var(--muted-foreground)]">Applied {formatDate(m.applied_at)}</div>
                  </div>
                </div>
                <div className="text-[10px] font-mono text-[var(--muted-foreground)] px-2 py-1 bg-[var(--background)] rounded border border-[var(--border)]">
                  {m.version}
                </div>
              </div>
            ))}
            {migrations?.applied.length === 0 && (
              <div className="py-12 text-center border-2 border-dashed border-[var(--border)] rounded-xl">
                <History size={32} className="mx-auto text-[var(--muted-foreground)]/20 mb-2" />
                <p className="text-sm text-[var(--muted-foreground)] italic">No migration history found.</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
