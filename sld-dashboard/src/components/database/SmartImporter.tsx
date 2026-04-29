import { useState, useRef } from "react";
import { 
  FileUp, 
  Table as TableIcon, 
  ArrowRight, 
  CheckCircle2, 
  Upload,
  Database,
  RefreshCw,
  X
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useTables, useTableColumns, useImportDataMutation } from "@/hooks/use-database";

interface SmartImporterProps {
  database: string;
}

export function SmartImporter({ database }: SmartImporterProps) {
  const { data: tables = [] } = useTables(database);
  const [selectedTable, setSelectedTable] = useState<string>("");
  const { data: columns = [] } = useTableColumns(database, selectedTable);
  const importMutation = useImportDataMutation();
  
  const [fileData, setFileData] = useState<any[]>([]);
  const [fileHeaders, setFileHeaders] = useState<string[]>([]);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [step, setStep] = useState<1 | 2 | 3>(1);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      const content = event.target?.result as string;
      if (file.name.endsWith('.json')) {
        const json = JSON.parse(content);
        const data = Array.isArray(json) ? json : [json];
        setFileData(data);
        setFileHeaders(Object.keys(data[0] || {}));
      } else {
        const lines = content.split('\n');
        const headers = lines[0].split(',').map(h => h.trim());
        const data = lines.slice(1).filter(l => l.trim()).map(line => {
          const values = line.split(',');
          return headers.reduce((obj, header, index) => {
            obj[header] = values[index]?.trim();
            return obj;
          }, {} as any);
        });
        setFileData(data);
        setFileHeaders(headers);
      }
      setStep(2);
    };
    reader.readAsText(file);
  };

  const handleImport = async () => {
    await importMutation.mutateAsync({
      database,
      table: selectedTable,
      mapping,
      data: fileData
    });
    setStep(3);
  };

  const autoMap = () => {
    const newMapping: Record<string, string> = {};
    fileHeaders.forEach(fh => {
      const match = columns.find(c => c.name.toLowerCase() === fh.toLowerCase());
      if (match) newMapping[fh] = match.name;
    });
    setMapping(newMapping);
  };

  return (
    <div className="max-w-4xl mx-auto space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <FileUp className="text-[var(--primary)]" /> Smart Data Importer
          </h2>
          <p className="text-[var(--muted-foreground)]">Map external files to your database schema with ease</p>
        </div>
      </div>

      {step === 1 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <Card className="border-[var(--border)] hover:border-[var(--primary)]/50 transition-colors cursor-pointer group" onClick={() => fileInputRef.current?.click()}>
            <CardContent className="p-12 flex flex-col items-center justify-center text-center space-y-4">
              <div className="p-4 rounded-full bg-[var(--primary)]/10 text-[var(--primary)] group-hover:scale-110 transition-transform">
                <Upload size={32} />
              </div>
              <div>
                <h3 className="text-lg font-bold">Upload File</h3>
                <p className="text-sm text-[var(--muted-foreground)]">Supports CSV and JSON formats</p>
              </div>
              <input 
                type="file" 
                ref={fileInputRef} 
                className="hidden" 
                accept=".csv,.json" 
                onChange={handleFileUpload} 
              />
            </CardContent>
          </Card>

          <Card className="border-[var(--border)]">
            <CardHeader>
              <CardTitle className="text-sm font-bold flex items-center gap-2">
                <TableIcon size={16} /> Destination Table
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
               <div className="space-y-2">
                  {tables.map(t => (
                    <button
                      key={t.name}
                      onClick={() => setSelectedTable(t.name)}
                      className={`w-full text-left px-4 py-2 rounded-lg text-sm transition-all ${
                        selectedTable === t.name 
                          ? 'bg-[var(--primary)] text-white shadow-lg' 
                          : 'hover:bg-[var(--muted)]/50'
                      }`}
                    >
                      {t.name}
                    </button>
                  ))}
               </div>
            </CardContent>
          </Card>
        </div>
      )}

      {step === 2 && (
        <Card className="border-[var(--border)] overflow-hidden shadow-2xl">
          <CardHeader className="bg-[var(--muted)]/20 border-b border-[var(--border)] flex flex-row items-center justify-between">
            <div>
              <CardTitle className="text-lg flex items-center gap-2">
                <Database size={18} /> Mapping Columns: {selectedTable}
              </CardTitle>
              <CardDescription>{fileData.length} rows detected in source file</CardDescription>
            </div>
            <Button variant="secondary" size="sm" onClick={autoMap} className="gap-2">
              <RefreshCw size={14} /> Auto-Map
            </Button>
          </CardHeader>
          <CardContent className="p-0">
            <table className="w-full text-sm">
              <thead className="bg-[var(--muted)]/50 text-[10px] uppercase font-bold text-[var(--muted-foreground)]">
                <tr>
                  <th className="px-6 py-3 text-left">Source File Column</th>
                  <th className="px-6 py-3 text-center w-20"></th>
                  <th className="px-6 py-3 text-left">Destination Database Column</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border)]">
                {fileHeaders.map(header => (
                  <tr key={header} className="hover:bg-[var(--muted)]/20 transition-colors">
                    <td className="px-6 py-4 font-mono font-bold text-blue-500">{header}</td>
                    <td className="px-6 py-4 text-center">
                       <ArrowRight size={14} className="mx-auto text-[var(--muted-foreground)]" />
                    </td>
                    <td className="px-6 py-4">
                      <select 
                        value={mapping[header] || ""} 
                        onChange={(e) => setMapping({...mapping, [header]: e.target.value})}
                        className="w-full bg-[var(--card)] border border-[var(--border)] rounded px-3 py-1.5 text-sm focus:ring-2 ring-[var(--primary)]/20 outline-none"
                      >
                        <option value="">Ignore Column</option>
                        {columns.map(c => (
                          <option key={c.name} value={c.name}>{c.name} ({c.type})</option>
                        ))}
                      </select>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
          <div className="p-6 bg-[var(--muted)]/20 border-t border-[var(--border)] flex justify-between items-center">
            <Button variant="ghost" onClick={() => setStep(1)} className="gap-2">
              <X size={14} /> Cancel
            </Button>
            <Button 
              onClick={handleImport} 
              disabled={Object.keys(mapping).length === 0 || importMutation.isPending}
              loading={importMutation.isPending}
              className="gap-2"
            >
              <CheckCircle2 size={14} /> Finalize Import
            </Button>
          </div>
        </Card>
      )}

      {step === 3 && (
        <Card className="border-green-500/20 bg-green-500/5 p-12 text-center space-y-4">
          <div className="mx-auto w-16 h-16 rounded-full bg-green-500/10 text-green-500 flex items-center justify-center">
            <CheckCircle2 size={32} />
          </div>
          <div className="space-y-2">
            <h3 className="text-2xl font-bold text-green-500">Import Successful!</h3>
            <p className="text-[var(--muted-foreground)]">Successfully processed and inserted {fileData.length} records into <span className="font-mono text-[var(--foreground)] font-bold">{selectedTable}</span>.</p>
          </div>
          <div className="pt-6">
            <Button onClick={() => { setStep(1); setFileData([]); setMapping({}); setSelectedTable(""); }}>
              Start New Import
            </Button>
          </div>
        </Card>
      )}
    </div>
  );
}
