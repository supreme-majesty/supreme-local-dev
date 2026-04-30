import { useState, useRef } from "react";
import { 
  Upload, 
  FileText, 
  Check, 
  ChevronRight, 
  ChevronLeft, 
  ArrowRight, 
  Table as TableIcon,
  Database,
  AlertCircle,
  Loader2,
  X
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { 
  useAnalyzeImportMutation, 
  useExecuteImportMutation,
  useTableColumns
} from "@/hooks/use-database";
import type { ImportAnalysis } from "@/api/daemon";

interface ImportWizardProps {
  database: string;
  table: string;
  onClose: () => void;
}

type Step = 'upload' | 'mapping' | 'preview' | 'executing' | 'complete';

export function ImportWizard({ database, table, onClose }: ImportWizardProps) {
  const [step, setStep] = useState<Step>('upload');
  const [file, setFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState<ImportAnalysis | null>(null);
  const [tempPath, setTempPath] = useState<string | null>(null);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const fileInputRef = useRef<HTMLInputElement>(null);

  const { data: tableCols } = useTableColumns(database, table);
  const analyzeMutation = useAnalyzeImportMutation();
  const executeMutation = useExecuteImportMutation();

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = e.target.files?.[0];
    if (!selectedFile) return;

    setFile(selectedFile);
    try {
      const result = await analyzeMutation.mutateAsync({ file: selectedFile });
      setAnalysis(result.analysis);
      setTempPath(result.temp_path);
      
      // Auto-map columns with same name
      const initialMapping: Record<string, string> = {};
      result.analysis.columns.forEach(col => {
        const match = tableCols?.find(tc => tc.name.toLowerCase() === col.toLowerCase());
        if (match) initialMapping[col] = match.name;
      });
      setMapping(initialMapping);
      
      setStep('mapping');
    } catch {
      console.error("Analysis failed");
    }
  };

  const handleExecute = async () => {
    if (!tempPath || !analysis) return;
    
    setStep('executing');
    try {
      await executeMutation.mutateAsync({
        database,
        table,
        temp_path: tempPath,
        format: analysis.format,
        mapping
      });
      setStep('complete');
    } catch {
      setStep('mapping');
    }
  };

  const renderUpload = () => (
    <div className="flex flex-col items-center justify-center py-12 border-2 border-dashed border-[var(--border)] rounded-xl bg-[var(--muted)]/20 hover:bg-[var(--muted)]/40 transition-all group cursor-pointer"
         onClick={() => fileInputRef.current?.click()}>
      <input 
        type="file" 
        ref={fileInputRef} 
        onChange={handleFileSelect} 
        className="hidden" 
        accept=".csv,.json"
      />
      <div className="w-16 h-16 rounded-full bg-[var(--primary)]/10 flex items-center justify-center mb-4 group-hover:scale-110 transition-transform">
        {analyzeMutation.isPending ? (
          <Loader2 className="animate-spin text-[var(--primary)]" size={32} />
        ) : (
          <Upload className="text-[var(--primary)]" size={32} />
        )}
      </div>
      <h3 className="text-lg font-semibold mb-2">Choose Import File</h3>
      <p className="text-[var(--muted-foreground)] text-sm mb-6">Support CSV and JSON formats up to 50MB</p>
      <Button variant="outline" className="gap-2">
        <FileText size={16} /> Select File
      </Button>
    </div>
  );

  const renderMapping = () => (
    <div className="space-y-6 animate-in fade-in slide-in-from-right-4 duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">Map Columns</h3>
          <p className="text-sm text-[var(--muted-foreground)]">Map file columns to table structure</p>
        </div>
        <div className="text-xs bg-[var(--primary)]/10 text-[var(--primary)] px-2 py-1 rounded-full font-medium">
          {file?.name}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 max-h-[400px] overflow-auto pr-2">
        {analysis?.columns.map((col) => (
          <div key={col} className="flex items-center gap-4 p-3 rounded-lg bg-[var(--muted)]/30 border border-[var(--border)]/50">
            <div className="flex-1">
              <div className="text-sm font-medium flex items-center gap-2">
                <FileText size={14} className="text-blue-400" />
                {col}
              </div>
              <div className="text-[10px] text-[var(--muted-foreground)] mt-1 italic">
                Sample: {String(analysis.preview[0]?.[col] || 'null')}
              </div>
            </div>
            <ArrowRight size={14} className="text-[var(--muted-foreground)]" />
            <div className="flex-1">
              <select 
                value={mapping[col] || ''} 
                onChange={(e) => setMapping({...mapping, [col]: e.target.value})}
                className="w-full bg-[var(--background)] border border-[var(--border)] rounded px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-[var(--primary)]"
              >
                <option value="">Ignore column</option>
                {tableCols?.map(tc => (
                  <option key={tc.name} value={tc.name}>{tc.name} ({tc.type})</option>
                ))}
              </select>
            </div>
          </div>
        ))}
      </div>

      <div className="flex justify-between pt-4">
        <Button variant="ghost" onClick={() => setStep('upload')} className="gap-2">
          <ChevronLeft size={16} /> Change File
        </Button>
        <Button onClick={() => setStep('preview')} className="gap-2">
          Preview Data <ChevronRight size={16} />
        </Button>
      </div>
    </div>
  );

  const renderPreview = () => (
    <div className="space-y-6 animate-in fade-in slide-in-from-right-4 duration-300">
      <div>
        <h3 className="text-lg font-semibold">Preview & Confirm</h3>
        <p className="text-sm text-[var(--muted-foreground)]">Review mapped data before inserting</p>
      </div>

      <div className="overflow-x-auto border border-[var(--border)] rounded-xl bg-[var(--card)]">
        <table className="w-full text-sm">
          <thead className="bg-[var(--muted)]/50 border-b border-[var(--border)]">
            <tr>
              {Object.values(mapping).filter(v => v !== '').map(col => (
                <th key={col} className="px-4 py-2 text-left font-medium text-[var(--muted-foreground)]">{col}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {analysis?.preview.map((row, i) => (
              <tr key={i} className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--muted)]/30">
                {Object.entries(mapping).filter(([, v]) => v !== '').map(([src]) => (
                  <td key={src} className="px-4 py-2 whitespace-nowrap">{String(row[src] || '')}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="flex justify-between pt-4">
        <Button variant="ghost" onClick={() => setStep('mapping')} className="gap-2">
          <ChevronLeft size={16} /> Back to Mapping
        </Button>
        <Button onClick={handleExecute} className="gap-2 bg-green-600 hover:bg-green-700 text-white">
          Start Import <Check size={16} />
        </Button>
      </div>
    </div>
  );

  const renderExecuting = () => (
    <div className="py-12 flex flex-col items-center justify-center space-y-6">
      <div className="relative">
        <div className="w-24 h-24 rounded-full border-4 border-[var(--primary)]/20 border-t-[var(--primary)] animate-spin" />
        <div className="absolute inset-0 flex items-center justify-center">
          <Database size={32} className="text-[var(--primary)]" />
        </div>
      </div>
      <div className="text-center">
        <h3 className="text-xl font-bold mb-2">Importing Data...</h3>
        <p className="text-[var(--muted-foreground)]">Writing rows to <span className="text-[var(--foreground)] font-mono">{table}</span></p>
        <p className="text-xs mt-4 text-yellow-500 flex items-center justify-center gap-1">
          <AlertCircle size={12} /> Please do not close this window
        </p>
      </div>
    </div>
  );

  const renderComplete = () => (
    <div className="py-12 flex flex-col items-center justify-center space-y-6 animate-in zoom-in-95 duration-500">
      <div className="w-20 h-20 rounded-full bg-green-500/10 flex items-center justify-center text-green-500">
        <Check size={48} />
      </div>
      <div className="text-center">
        <h3 className="text-2xl font-bold mb-2">Import Complete!</h3>
        <p className="text-[var(--muted-foreground)]">Successfully imported data from <span className="text-[var(--foreground)] font-semibold">{file?.name}</span></p>
      </div>
      <Button onClick={onClose} className="min-w-[200px]">Close Wizard</Button>
    </div>
  );

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-in fade-in duration-200">
      <Card className="w-full max-w-2xl bg-[var(--card)] border-[var(--border)] shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--border)] bg-[var(--muted)]/30">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-[var(--primary)]/10 text-[var(--primary)]">
              <Upload size={20} />
            </div>
            <div>
              <h2 className="text-xl font-bold">Import Wizard</h2>
              <div className="text-xs text-[var(--muted-foreground)] flex items-center gap-1">
                <Database size={10} /> {database} <ChevronRight size={8} /> <TableIcon size={10} /> {table}
              </div>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-[var(--muted)] rounded-full transition-colors">
            <X size={20} className="text-[var(--muted-foreground)]" />
          </button>
        </div>

        <CardContent className="p-6">
          {/* Steps Indicator */}
          {step !== 'complete' && step !== 'executing' && (
            <div className="flex items-center justify-center mb-8">
              {[
                { id: 'upload', label: 'Upload' },
                { id: 'mapping', label: 'Map' },
                { id: 'preview', label: 'Confirm' }
              ].map((s, i, arr) => (
                <div key={s.id} className="flex items-center">
                  <div className={`flex flex-col items-center group`}>
                    <div className={`w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold transition-all
                      ${step === s.id ? 'bg-[var(--primary)] text-white scale-110 shadow-lg shadow-[var(--primary)]/20' : 
                        arr.findIndex(x => x.id === step) > i ? 'bg-green-500 text-white' : 'bg-[var(--muted)] text-[var(--muted-foreground)]'}`}>
                      {arr.findIndex(x => x.id === step) > i ? <Check size={14} /> : i + 1}
                    </div>
                    <span className={`text-[10px] mt-1 font-medium ${step === s.id ? 'text-[var(--primary)]' : 'text-[var(--muted-foreground)]'}`}>
                      {s.label}
                    </span>
                  </div>
                  {i < arr.length - 1 && (
                    <div className={`w-16 h-px mx-2 transition-colors ${arr.findIndex(x => x.id === step) > i ? 'bg-green-500' : 'bg-[var(--border)]'}`} />
                  )}
                </div>
              ))}
            </div>
          )}

          {step === 'upload' && renderUpload()}
          {step === 'mapping' && renderMapping()}
          {step === 'preview' && renderPreview()}
          {step === 'executing' && renderExecuting()}
          {step === 'complete' && renderComplete()}
        </CardContent>
      </Card>
    </div>
  );
}
