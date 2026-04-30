/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState } from "react";
import { 
  Server, 
  Plus, 
  Settings, 
  CheckCircle2, 
  Globe, 
  ShieldAlert, 
  ChevronRight,
  Database,
  Trash2,
  Activity
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Badge } from "@/components/ui/Badge";
import { useProfiles, useSaveProfileMutation, useSwitchEnvironmentMutation } from "@/hooks/use-database";

export function EnvironmentManager() {
  const { data: profiles = [] } = useProfiles();
  const saveMutation = useSaveProfileMutation();
  const switchMutation = useSwitchEnvironmentMutation();
  
  const [isAdding, setIsAdding] = useState(false);
  const [newProfile, setNewProfile] = useState({
    name: "",
    host: "localhost",
    port: "3306",
    user: "root",
    password: "",
    environment: "DEVELOPMENT"
  });

  const handleSave = () => {
    saveMutation.mutate({ ...newProfile, id: Date.now().toString() });
    setIsAdding(false);
  };

  const getEnvColor = (env: string) => {
    switch (env) {
      case 'PRODUCTION': return 'text-red-500 bg-red-500/10 border-red-500/20';
      case 'STAGING': return 'text-amber-500 bg-amber-500/10 border-amber-500/20';
      default: return 'text-blue-500 bg-blue-500/10 border-blue-500/20';
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <Globe className="text-blue-500" /> Environment Orchestrator
          </h2>
          <p className="text-[var(--muted-foreground)]">Manage connection profiles and multi-environment synchronization</p>
        </div>
        <Button onClick={() => setIsAdding(true)} className="gap-2">
          <Plus size={16} /> Add Environment
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {/* Local Environment (Default) */}
        <Card className="border-[var(--primary)]/30 bg-[var(--primary)]/5 relative overflow-hidden group">
          <div className="absolute top-0 right-0 p-2 opacity-10 group-hover:opacity-20 transition-opacity">
            <Server size={80} />
          </div>
          <CardHeader>
            <div className="flex items-center justify-between">
              <Badge className="bg-blue-500/20 text-blue-500 border-blue-500/30">CURRENT</Badge>
              <Badge className={getEnvColor('DEVELOPMENT')}>DEVELOPMENT</Badge>
            </div>
            <CardTitle className="mt-4 flex items-center gap-2">
              Local Instance
            </CardTitle>
            <CardDescription>localhost:3306</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex items-center justify-between text-xs text-[var(--muted-foreground)]">
                <span>Latency</span>
                <span className="text-green-500 font-mono">1.2ms</span>
              </div>
              <div className="h-1.5 w-full bg-black/20 rounded-full overflow-hidden">
                <div className="h-full bg-green-500 w-[98%]" />
              </div>
              <Button variant="outline" size="sm" className="w-full gap-2" disabled>
                <CheckCircle2 size={14} /> Active Environment
              </Button>
            </div>
          </CardContent>
        </Card>

        {/* Dynamic Profiles */}
        {profiles.map((p: any) => (
          <Card key={p.id} className="border-[var(--border)] hover:border-[var(--primary)]/50 transition-all group relative overflow-hidden">
            <div className="absolute top-0 right-0 p-2 opacity-10 group-hover:opacity-20 transition-opacity">
              <Server size={80} />
            </div>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-1">
                  {p.environment === 'PRODUCTION' && <ShieldAlert size={14} className="text-red-500" />}
                  <Badge className={getEnvColor(p.environment)}>{p.environment}</Badge>
                </div>
                <Button variant="ghost" size="icon" className="h-8 w-8 text-red-500/50 hover:text-red-500">
                  <Trash2 size={14} />
                </Button>
              </div>
              <CardTitle className="mt-4">{p.name}</CardTitle>
              <CardDescription>{p.host}:{p.port}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-center justify-between text-xs text-[var(--muted-foreground)]">
                  <span>Last Sync</span>
                  <span className="font-mono">Never</span>
                </div>
                <div className="h-1.5 w-full bg-black/10 rounded-full" />
                <Button 
                  onClick={() => switchMutation.mutate(p.id)}
                  variant="secondary" 
                  size="sm" 
                  className="w-full gap-2 group-hover:bg-[var(--primary)] group-hover:text-white transition-colors"
                  disabled={switchMutation.isPending}
                >
                  <Activity size={14} /> Switch to {p.name}
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}

        {/* Add New Profile Form */}
        {isAdding && (
          <Card className="border-dashed border-[var(--primary)]/50 bg-[var(--primary)]/5">
            <CardHeader>
              <CardTitle className="text-sm">New Connection Profile</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Input 
                placeholder="Environment Name (e.g. Staging DB)" 
                value={newProfile.name}
                onChange={(e) => setNewProfile({...newProfile, name: e.target.value})}
              />
              <div className="grid grid-cols-2 gap-2">
                <Input 
                  placeholder="Host" 
                  value={newProfile.host}
                  onChange={(e) => setNewProfile({...newProfile, host: e.target.value})}
                />
                <Input 
                  placeholder="Port" 
                  value={newProfile.port}
                  onChange={(e) => setNewProfile({...newProfile, port: e.target.value})}
                />
              </div>
              <select 
                className="w-full h-10 px-3 rounded-md border border-[var(--border)] bg-[var(--background)] text-sm"
                value={newProfile.environment}
                onChange={(e) => setNewProfile({...newProfile, environment: e.target.value})}
              >
                <option value="DEVELOPMENT">Development</option>
                <option value="STAGING">Staging</option>
                <option value="PRODUCTION">Production</option>
              </select>
              <div className="flex gap-2">
                <Button size="sm" className="flex-1" onClick={handleSave}>Save</Button>
                <Button size="sm" variant="ghost" onClick={() => setIsAdding(false)}>Cancel</Button>
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Advanced Orchestration Tools */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="border-[var(--border)]">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <CheckCircle2 size={18} className="text-green-500" /> Schema Promotion
            </CardTitle>
            <CardDescription>Compare and synchronize structures between environments</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="p-8 border border-dashed border-[var(--border)] rounded-xl text-center bg-[var(--muted)]/20">
               <ShieldAlert size={32} className="mx-auto text-[var(--muted-foreground)]/30 mb-4" />
               <p className="text-sm text-[var(--muted-foreground)]">Select two environments to start a promotion script</p>
               <Button variant="outline" size="sm" className="mt-4 gap-2">
                 Launch Sync Tool <ChevronRight size={14} />
               </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="border-[var(--border)]">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <Activity size={18} className="text-blue-500" /> Maintenance Scheduler
            </CardTitle>
            <CardDescription>Automate backups and optimizations</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex items-center justify-between p-3 bg-[var(--muted)]/30 rounded-lg border border-[var(--border)]">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-blue-500/10 rounded-lg"><Database size={16} className="text-blue-500" /></div>
                  <div>
                    <div className="text-sm font-bold">Nightly Backup</div>
                    <div className="text-[10px] text-[var(--muted-foreground)]">Runs daily at 02:00 AM</div>
                  </div>
                </div>
                <Badge variant="outline" className="text-green-500 border-green-500/20">ENABLED</Badge>
              </div>
              <div className="flex items-center justify-between p-3 bg-[var(--muted)]/30 rounded-lg border border-[var(--border)]">
                <div className="flex items-center gap-3">
                  <div className="p-2 bg-amber-500/10 rounded-lg"><Settings size={16} className="text-amber-500" /></div>
                  <div>
                    <div className="text-sm font-bold">Index Optimization</div>
                    <div className="text-[10px] text-[var(--muted-foreground)]">Runs every Sunday</div>
                  </div>
                </div>
                <Badge variant="outline" className="text-[var(--muted-foreground)]">DISABLED</Badge>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
