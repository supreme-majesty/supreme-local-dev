import { useState } from "react";
import { 
  Zap, 
  Database,
  Loader2,
  X,
  Sparkles,
  ChevronRight,
  Trash2
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { 
  useSeedTableMutation,
  useTableColumns
} from "@/hooks/use-database";

interface DataSeederProps {
  database: string;
  table: string;
  onClose: () => void;
}

const FAKER_TYPES = [
  { id: 'name', label: 'Full Name' },
  { id: 'email', label: 'Email Address' },
  { id: 'phone', label: 'Phone Number' },
  { id: 'address', label: 'Street Address' },
  { id: 'city', label: 'City' },
  { id: 'country', label: 'Country' },
  { id: 'company', label: 'Company Name' },
  { id: 'job_title', label: 'Job Title' },
  { id: 'date', label: 'Date (YYYY-MM-DD)' },
  { id: 'datetime', label: 'DateTime' },
  { id: 'sentence', label: 'Short Sentence' },
  { id: 'paragraph', label: 'Paragraph' },
  { id: 'number', label: 'Random Integer' },
  { id: 'float', label: 'Random Float' },
  { id: 'bool', label: 'Boolean' },
  { id: 'uuid', label: 'UUID' },
  { id: 'color', label: 'Hex Color' },
  { id: 'username', label: 'Username' },
  { id: 'password', label: 'Secure Password' },
];

export function DataSeeder({ database, table, onClose }: DataSeederProps) {
  const [count, setCount] = useState(10);
  const [fakers, setFakers] = useState<Record<string, string>>({});
  const [isSeeding, setIsSeeding] = useState(false);

  const { data: tableCols } = useTableColumns(database, table);
  const seedMutation = useSeedTableMutation();

  const handleSeed = async () => {
    if (Object.keys(fakers).length === 0) return;
    
    setIsSeeding(true);
    try {
      await seedMutation.mutateAsync({
        database,
        table,
        count,
        fakers
      });
      onClose();
    } catch (err) {
      console.error("Seeding failed", err);
    } finally {
      setIsSeeding(false);
    }
  };

  const autoMatch = () => {
    const newFakers: Record<string, string> = {};
    tableCols?.forEach(col => {
      const name = col.name.toLowerCase();
      if (name.includes('email')) newFakers[col.name] = 'email';
      else if (name.includes('name')) newFakers[col.name] = 'name';
      else if (name.includes('phone')) newFakers[col.name] = 'phone';
      else if (name.includes('address')) newFakers[col.name] = 'address';
      else if (name.includes('city')) newFakers[col.name] = 'city';
      else if (name.includes('country')) newFakers[col.name] = 'country';
      else if (name.includes('company')) newFakers[col.name] = 'company';
      else if (name.includes('date')) newFakers[col.name] = 'date';
      else if (name.includes('created_at') || name.includes('updated_at')) newFakers[col.name] = 'datetime';
      else if (name.includes('uuid')) newFakers[col.name] = 'uuid';
      else if (name.includes('desc') || name.includes('bio')) newFakers[col.name] = 'paragraph';
      else if (name.includes('title') || name.includes('subject')) newFakers[col.name] = 'sentence';
    });
    setFakers(newFakers);
  };

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-in fade-in duration-200">
      <Card className="w-full max-w-2xl bg-[var(--card)] border-[var(--border)] shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--border)] bg-indigo-500/10">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-indigo-500 text-white">
              <Zap size={20} />
            </div>
            <div>
              <h2 className="text-xl font-bold">Mock Data Generator</h2>
              <div className="text-xs text-[var(--muted-foreground)] flex items-center gap-1">
                <Database size={10} /> {database} <ChevronRight size={8} /> {table}
              </div>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-[var(--muted)] rounded-full transition-colors">
            <X size={20} className="text-[var(--muted-foreground)]" />
          </button>
        </div>

        <CardContent className="p-6">
          <div className="space-y-6">
            <div className="flex items-end gap-4 bg-[var(--muted)]/20 p-4 rounded-xl border border-[var(--border)]">
              <div className="flex-1 space-y-1.5">
                <label className="text-xs font-semibold text-[var(--muted-foreground)] uppercase tracking-wider">Number of Rows</label>
                <div className="flex items-center gap-2">
                  <input 
                    type="number" 
                    value={count} 
                    onChange={(e) => setCount(Math.max(1, parseInt(e.target.value) || 1))}
                    className="w-full bg-[var(--background)] border border-[var(--border)] rounded-lg px-3 py-2 font-mono text-lg focus:ring-2 focus:ring-indigo-500 outline-none"
                  />
                </div>
              </div>
              <Button variant="outline" className="gap-2 h-[46px]" onClick={autoMatch}>
                <Sparkles size={16} className="text-yellow-500" /> Auto-Match
              </Button>
            </div>

            <div className="space-y-3">
              <h3 className="text-sm font-semibold flex items-center gap-2">
                Configure Generators
                <span className="text-[10px] bg-[var(--muted)] px-2 py-0.5 rounded text-[var(--muted-foreground)] font-normal">
                  {Object.keys(fakers).length} columns configured
                </span>
              </h3>
              
              <div className="space-y-2 max-h-[350px] overflow-auto pr-2">
                {tableCols?.filter(col => col.key !== 'PRI' || !col.extra?.includes('auto_increment')).map(col => (
                  <div key={col.name} className="flex items-center gap-3 p-3 rounded-lg bg-[var(--muted)]/10 border border-[var(--border)]/50 hover:border-indigo-500/50 transition-colors">
                    <div className="flex-1">
                      <div className="text-sm font-medium">{col.name}</div>
                      <div className="text-[10px] text-[var(--muted-foreground)] uppercase">{col.type} {col.nullable ? '' : '(Required)'}</div>
                    </div>
                    
                    <select 
                      value={fakers[col.name] || ''} 
                      onChange={(e) => {
                        const val = e.target.value;
                        if (!val) {
                          const newFakers = {...fakers};
                          delete newFakers[col.name];
                          setFakers(newFakers);
                        } else {
                          setFakers({...fakers, [col.name]: val});
                        }
                      }}
                      className="flex-1 bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1.5 text-sm focus:ring-1 focus:ring-indigo-500 outline-none"
                    >
                      <option value="">No generator</option>
                      {FAKER_TYPES.map(f => (
                        <option key={f.id} value={f.id}>{f.label}</option>
                      ))}
                    </select>

                    {fakers[col.name] && (
                      <button 
                        onClick={() => {
                          const newFakers = {...fakers};
                          delete newFakers[col.name];
                          setFakers(newFakers);
                        }}
                        className="p-1.5 text-[var(--muted-foreground)] hover:text-red-500 transition-colors"
                      >
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div className="flex justify-between items-center pt-4 border-t border-[var(--border)]">
              <div className="text-xs text-[var(--muted-foreground)]">
                {Object.keys(fakers).length === 0 ? (
                  <span className="flex items-center gap-1 text-yellow-500">
                    <AlertCircle size={12} /> Select at least one generator
                  </span>
                ) : (
                  <span>Will generate {count} rows into {table}</span>
                )}
              </div>
              <div className="flex gap-3">
                <Button variant="ghost" onClick={onClose}>Cancel</Button>
                <Button 
                  onClick={handleSeed} 
                  disabled={isSeeding || Object.keys(fakers).length === 0}
                  className="bg-indigo-600 hover:bg-indigo-700 text-white gap-2 min-w-[140px]"
                >
                  {isSeeding ? (
                    <Loader2 className="animate-spin" size={16} />
                  ) : (
                    <>Generate Data <Zap size={16} /></>
                  )}
                </Button>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

const AlertCircle = ({ size }: { size: number }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
);
