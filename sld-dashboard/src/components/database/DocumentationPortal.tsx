import { useState } from "react";
import { 
  BookOpen, 
  Search, 
  FileText, 
  Download, 
  Layers, 
  Key,
  Info,
  ExternalLink,
  ChevronRight,
  Printer
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Badge } from "@/components/ui/Badge";
import { useSchemaDocs } from "@/hooks/use-database";

interface DocumentationPortalProps {
  database: string;
}

export function DocumentationPortal({ database }: DocumentationPortalProps) {
  const { data: docs = [], isLoading } = useSchemaDocs(database);
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedTable, setSelectedTable] = useState<string | null>(null);

  const filteredDocs = docs.filter((t: any) => 
    t.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    t.columns.some((c: any) => c.name.toLowerCase().includes(searchTerm.toLowerCase()))
  );

  const activeTable = selectedTable ? docs.find((t: any) => t.name === selectedTable) : filteredDocs[0];

  if (isLoading) return (
    <div className="flex flex-col items-center justify-center p-20 animate-pulse">
      <BookOpen size={48} className="text-[var(--primary)]/20 mb-4" />
      <p className="text-[var(--muted-foreground)] italic">Generating schema documentation...</p>
    </div>
  );

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-3">
            <BookOpen className="text-blue-500" /> Schema Documentation
          </h2>
          <p className="text-[var(--muted-foreground)]">Automatically generated data dictionary for <span className="font-mono text-[var(--foreground)]">{database}</span></p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" size="sm" className="gap-2" onClick={() => window.print()}>
            <Printer size={14} /> Print PDF
          </Button>
          <Button size="sm" className="gap-2">
            <Download size={14} /> Export Markdown
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-8">
        <div className="lg:col-span-1 space-y-4">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]" size={14} />
            <Input 
              placeholder="Search tables or columns..." 
              className="pl-9"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
            />
          </div>
          <div className="space-y-1 max-h-[600px] overflow-y-auto pr-2 custom-scrollbar">
            {filteredDocs.map((t: any) => (
              <button
                key={t.name}
                onClick={() => setSelectedTable(t.name)}
                className={`w-full text-left px-4 py-2.5 rounded-lg text-sm transition-all flex items-center justify-between group ${
                  activeTable?.name === t.name 
                    ? 'bg-blue-500/10 text-blue-500 font-bold border border-blue-500/20 shadow-sm' 
                    : 'hover:bg-[var(--muted)]/50 border border-transparent'
                }`}
              >
                <div className="flex items-center gap-3 truncate">
                  <Layers size={14} className={activeTable?.name === t.name ? 'text-blue-500' : 'text-[var(--muted-foreground)]'} />
                  <span className="truncate">{t.name}</span>
                </div>
                <ChevronRight size={12} className={`transition-transform ${activeTable?.name === t.name ? 'rotate-90' : 'opacity-0 group-hover:opacity-100'}`} />
              </button>
            ))}
          </div>
        </div>

        <div className="lg:col-span-3 space-y-6">
          {activeTable ? (
            <div className="space-y-6 animate-in fade-in slide-in-from-right-4 duration-300">
              <Card className="border-[var(--border)] overflow-hidden shadow-xl shadow-blue-500/5">
                <CardHeader className="bg-[var(--muted)]/20 border-b border-[var(--border)]">
                  <div className="flex items-start justify-between">
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className="bg-blue-500/10 text-blue-500 border-blue-500/20 uppercase text-[8px] font-bold">Table</Badge>
                        <CardTitle className="text-2xl font-mono">{activeTable.name}</CardTitle>
                      </div>
                      <CardDescription className="text-sm italic">{activeTable.description}</CardDescription>
                    </div>
                    <div className="flex gap-4 text-right">
                      <div>
                        <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase">Rows</div>
                        <div className="font-mono font-bold text-lg">{activeTable.row_count.toLocaleString()}</div>
                      </div>
                      <div className="w-px h-8 bg-[var(--border)]" />
                      <div>
                        <div className="text-[10px] font-bold text-[var(--muted-foreground)] uppercase">Storage</div>
                        <div className="font-mono font-bold text-lg">{activeTable.size}</div>
                      </div>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <table className="w-full text-sm">
                    <thead className="bg-[var(--muted)]/50 text-[10px] uppercase font-bold text-[var(--muted-foreground)] border-b border-[var(--border)]">
                      <tr>
                        <th className="px-6 py-3 text-left">Column</th>
                        <th className="px-6 py-3 text-left">Type</th>
                        <th className="px-6 py-3 text-left">Attributes</th>
                        <th className="px-6 py-3 text-left">Description</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-[var(--border)]">
                      {activeTable.columns.map((c: any) => (
                        <tr key={c.name} className="hover:bg-[var(--muted)]/20 transition-colors group">
                          <td className="px-6 py-4">
                            <div className="flex items-center gap-2">
                              <span className="font-mono font-bold text-[var(--foreground)]">{c.name}</span>
                              {c.key === 'PRI' && <Key size={12} className="text-amber-500" />}
                            </div>
                          </td>
                          <td className="px-6 py-4">
                            <code className="text-[10px] bg-[var(--muted)]/50 px-1.5 py-0.5 rounded text-blue-500">{c.type}</code>
                          </td>
                          <td className="px-6 py-4">
                            <div className="flex flex-wrap gap-1">
                              {!c.is_nullable && <Badge className="bg-red-500/10 text-red-500 border-red-500/20 text-[8px] uppercase">NOT NULL</Badge>}
                              {c.default && <Badge variant="outline" className="text-[8px] uppercase">DEF: {c.default}</Badge>}
                            </div>
                          </td>
                          <td className="px-6 py-4">
                            <p className="text-xs text-[var(--muted-foreground)] group-hover:text-[var(--foreground)] transition-colors">{c.description}</p>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </CardContent>
              </Card>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <Card className="border-[var(--border)] bg-amber-500/5 border-amber-500/20">
                  <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-bold flex items-center gap-2 text-amber-500">
                      <Info size={16} /> Knowledge Base
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <p className="text-xs text-[var(--muted-foreground)] leading-relaxed">
                      This table serves as the primary source of truth for **{activeTable.name}** entities. It is frequently joined with other core tables and indexed on high-cardinality columns for optimal retrieval performance.
                    </p>
                  </CardContent>
                </Card>
                <Card className="border-[var(--border)] bg-blue-500/5 border-blue-500/20">
                  <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-bold flex items-center gap-2 text-blue-500">
                      <ExternalLink size={16} /> Relationships
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="outline" className="gap-2 cursor-pointer hover:bg-blue-500/10 transition-colors">
                        <Layers size={10} /> Has Many Orders
                      </Badge>
                      <Badge variant="outline" className="gap-2 cursor-pointer hover:bg-blue-500/10 transition-colors">
                        <Layers size={10} /> Belongs To Region
                      </Badge>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </div>
          ) : (
            <div className="h-[500px] flex flex-col items-center justify-center border-2 border-dashed border-[var(--border)] rounded-2xl bg-[var(--muted)]/5">
              <FileText size={48} className="text-[var(--muted-foreground)]/20 mb-4" />
              <h3 className="text-lg font-semibold text-[var(--muted-foreground)]">Table Selection Required</h3>
              <p className="text-sm text-[var(--muted-foreground)]/60">Search and select a table on the left to view detailed documentation.</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
