import { useState } from "react";
import { 
  Shield, 
  Lock, 
  EyeOff, 
  AlertTriangle, 
  CheckCircle2, 
  RefreshCw,
  Search,
  Table as TableIcon,
  Fingerprint
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { usePIIScan, useMaskDataMutation, useTables } from "@/hooks/use-database";

interface SecurityDashboardProps {
  database: string;
}

export function SecurityDashboard({ database }: SecurityDashboardProps) {
  const { data: tables = [] } = useTables(database);
  const [selectedTable, setSelectedTable] = useState<string>("");
  const { data: piiResults, isLoading: scanning, refetch } = usePIIScan(database, selectedTable);
  const maskMutation = useMaskDataMutation();

  const handleMask = async () => {
    if (!selectedTable || !piiResults) return;
    
    const config = {
      database,
      table: selectedTable,
      columns: piiResults.reduce((acc: any, curr: any) => {
        acc[curr.column] = curr.pattern;
        return acc;
      }, {})
    };

    if (window.confirm(`Are you sure you want to permanently mask ${piiResults.length} columns in table '${selectedTable}'? This action cannot be undone.`)) {
      await maskMutation.mutateAsync(config);
      refetch();
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <Shield className="text-red-500" /> Security & Privacy
          </h2>
          <p className="text-[var(--muted-foreground)]">Scan and protect sensitive data in <span className="font-mono text-[var(--foreground)]">{database}</span></p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        <Card className="lg:col-span-1 border-[var(--border)]">
          <CardHeader>
            <CardTitle className="text-sm font-bold flex items-center gap-2">
              <TableIcon size={16} /> Select Table
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <div className="max-h-[500px] overflow-y-auto">
              {tables.map((t) => (
                <button
                  key={t.name}
                  onClick={() => setSelectedTable(t.name)}
                  className={`w-full text-left px-4 py-3 text-sm transition-colors flex items-center justify-between group ${
                    selectedTable === t.name 
                      ? 'bg-[var(--primary)]/10 text-[var(--primary)] font-bold border-r-2 border-[var(--primary)]' 
                      : 'hover:bg-[var(--muted)]/50'
                  }`}
                >
                  <span className="truncate">{t.name}</span>
                  {selectedTable === t.name && <Search size={14} />}
                </button>
              ))}
            </div>
          </CardContent>
        </Card>

        <div className="lg:col-span-3 space-y-6">
          {!selectedTable ? (
            <div className="h-[400px] flex flex-col items-center justify-center border-2 border-dashed border-[var(--border)] rounded-2xl bg-[var(--muted)]/5">
              <Fingerprint size={48} className="text-[var(--muted-foreground)]/20 mb-4" />
              <h3 className="text-lg font-semibold text-[var(--muted-foreground)]">PII Scanner Ready</h3>
              <p className="text-sm text-[var(--muted-foreground)]/60">Select a table to scan for personally identifiable information</p>
            </div>
          ) : scanning ? (
            <div className="h-[400px] flex flex-col items-center justify-center animate-pulse">
              <RefreshCw size={48} className="text-[var(--primary)]/20 mb-4 animate-spin" />
              <p className="text-[var(--muted-foreground)] font-medium text-lg">Scanning `{selectedTable}` for PII...</p>
              <p className="text-xs text-[var(--muted-foreground)]/60 mt-2">Checking patterns for emails, phones, and credit cards</p>
            </div>
          ) : (
            <>
              <Card className="border-[var(--border)] overflow-hidden shadow-xl shadow-red-500/5">
                <CardHeader className="bg-[var(--muted)]/20 border-b border-[var(--border)]">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-red-500/10 text-red-500">
                        <Lock size={20} />
                      </div>
                      <div>
                        <CardTitle className="text-lg">PII Scan Results: {selectedTable}</CardTitle>
                        <CardDescription>Detected {piiResults?.length || 0} sensitive columns</CardDescription>
                      </div>
                    </div>
                    {piiResults && piiResults.length > 0 && (
                      <Button 
                        onClick={handleMask} 
                        disabled={maskMutation.isPending}
                        className="bg-red-600 hover:bg-red-700 shadow-lg shadow-red-500/20 gap-2"
                      >
                        {maskMutation.isPending ? <RefreshCw size={14} className="animate-spin" /> : <EyeOff size={14} />}
                        Mask All Sensitive Data
                      </Button>
                    )}
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <div className="divide-y divide-[var(--border)]">
                    {piiResults?.map((res: any) => (
                      <div key={res.column} className="p-6 hover:bg-[var(--muted)]/10 transition-colors">
                        <div className="flex items-start justify-between">
                          <div className="space-y-3 flex-1">
                            <div className="flex items-center gap-3">
                              <span className="font-mono text-sm font-bold bg-[var(--card)] px-2 py-1 rounded border border-[var(--border)]">{res.column}</span>
                              <Badge variant="outline" className={
                                res.risk === 'HIGH' ? 'border-red-500 text-red-500 bg-red-500/5' :
                                res.risk === 'MEDIUM' ? 'border-orange-500 text-orange-500 bg-orange-500/5' :
                                'border-blue-500 text-blue-500 bg-blue-500/5'
                              }>
                                {res.risk} RISK
                              </Badge>
                              <span className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase tracking-widest">{res.pattern} Detected</span>
                            </div>
                            
                            <div className="flex flex-wrap gap-2 mt-4">
                              {res.examples.map((ex: string, i: number) => (
                                <div key={i} className="text-[10px] font-mono bg-[var(--muted)]/50 px-2 py-1 rounded border border-[var(--border)]/50">
                                  {ex}
                                </div>
                              ))}
                            </div>
                          </div>
                          
                          <div className="p-4 bg-amber-500/10 rounded-xl border border-amber-500/20 max-w-[200px]">
                            <div className="flex items-center gap-2 text-amber-500 font-bold text-[10px] uppercase mb-1">
                              <AlertTriangle size={12} /> Privacy Alert
                            </div>
                            <p className="text-[10px] text-[var(--muted-foreground)] leading-relaxed">
                              This column contains high-risk data. Anonymize this table before sharing with third parties.
                            </p>
                          </div>
                        </div>
                      </div>
                    ))}

                    {(!piiResults || piiResults.length === 0) && (
                      <div className="p-20 text-center">
                        <CheckCircle2 size={48} className="mx-auto text-green-500/20 mb-4" />
                        <h3 className="text-lg font-semibold text-[var(--muted-foreground)]">No PII Detected</h3>
                        <p className="text-sm text-[var(--muted-foreground)]/60">This table appears to be safe for general usage.</p>
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>

              {piiResults && piiResults.length > 0 && (
                <div className="p-4 bg-blue-500/5 border border-blue-500/20 rounded-xl flex items-start gap-4">
                  <Fingerprint className="text-blue-500 shrink-0" size={24} />
                  <div>
                    <h4 className="text-sm font-bold text-blue-500">Security Best Practice</h4>
                    <p className="text-xs text-[var(--muted-foreground)] leading-relaxed mt-1">
                      Always mask Personally Identifiable Information (PII) in local development environments to prevent accidental data leaks. The **Anonymize** tool uses high-entropy synthetic data generators to maintain schema realism while protecting individual privacy.
                    </p>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
