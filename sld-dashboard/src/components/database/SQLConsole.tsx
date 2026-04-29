import { useState, useEffect, useRef } from "react";
import { useMutation } from "@tanstack/react-query";
import { useTables } from "@/hooks/use-database";
import Editor from "@monaco-editor/react";
import {
  Play,
  Eraser,
  AlertCircle,
  History,
  FileText,
  Copy,
  Download,
  Clock,
  ChevronDown,
  Check,
  Database,
  Save,
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { QueryBuilder } from "./QueryBuilder";

interface SQLConsoleProps {
  database: string | null;
}

interface QueryResult {
  columns: string[];
  rows: Record<string, any>[];
  row_count: number;
  affected_rows?: number;
  execution_time_ms?: number;
}

interface QueryExplanation {
  analysis: string[];
  recommendations: string[];
  estimated_rows: number;
  complexity: string;
}

interface HistoryEntry {
  query: string;
  database: string;
  timestamp: number;
}

const HISTORY_KEY = "sld_sql_history";
const MAX_HISTORY = 50;

const SQL_TEMPLATES = [
  { label: "Select All", sql: "SELECT * FROM table_name LIMIT 100;" },
  {
    label: "Select Columns",
    sql: "SELECT col1, col2 FROM table_name WHERE condition;",
  },
  {
    label: "Insert Row",
    sql: "INSERT INTO table_name (col1, col2) VALUES ('val1', 'val2');",
  },
  {
    label: "Update Rows",
    sql: "UPDATE table_name SET col1 = 'value' WHERE condition;",
  },
  { label: "Delete Rows", sql: "DELETE FROM table_name WHERE condition;" },
  { label: "Show Create Table", sql: "SHOW CREATE TABLE table_name;" },
  { label: "Describe Table", sql: "DESCRIBE table_name;" },
  { label: "Show Tables", sql: "SHOW TABLES;" },
  { label: "Show Databases", sql: "SHOW DATABASES;" },
];

async function explainQuery(
  database: string,
  query: string
): Promise<QueryExplanation> {
  const res = await fetch("/api/db/explain", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, query }),
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Explain failed");
  }

  return res.json();
}

async function executeQuery(
  database: string,
  query: string
): Promise<QueryResult> {
  const startTime = performance.now();
  const res = await fetch("/api/db/query", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, query }),
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Query failed");
  }

  const result = await res.json();
  // Add client-side execution time if backend doesn't provide it
  if (!result.execution_time_ms) {
    result.execution_time_ms = Math.round(performance.now() - startTime);
  }
  return result;
}

function loadHistory(): HistoryEntry[] {
  try {
    const data = localStorage.getItem(HISTORY_KEY);
    return data ? JSON.parse(data) : [];
  } catch {
    return [];
  }
}

function saveHistory(history: HistoryEntry[]) {
  localStorage.setItem(
    HISTORY_KEY,
    JSON.stringify(history.slice(0, MAX_HISTORY))
  );
}

function formatTime(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function convertToCSV(columns: string[], rows: Record<string, any>[]): string {
  const escape = (val: string | number | boolean | null | undefined) => {
    if (val === null || val === undefined) return "";
    const str = String(val);
    if (str.includes(",") || str.includes('"') || str.includes("\n")) {
      return `"${str.replace(/"/g, '""')}"`;
    }
    return str;
  };
  const header = columns.map(escape).join(",");
  const body = rows
    .map((row) => columns.map((col) => escape(row[col])).join(","))
    .join("\n");
  return `${header}\n${body}`;
}

export function SQLConsole({ database }: SQLConsoleProps) {
  const [query, setQuery] = useState("");
  const [autoSave, setAutoSave] = useState(() => {
    return localStorage.getItem("sld_sql_autosave") === "true";
  });
  const [result, setResult] = useState<QueryResult | null>(null);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const [showTemplates, setShowTemplates] = useState(false);
  const [showBuilder, setShowBuilder] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const [editingCell, setEditingCell] = useState<{ rowIdx: number; col: string; value: string } | null>(null);
  
  const [explanation, setExplanation] = useState<QueryExplanation | null>(null);
  const [showExplanation, setShowExplanation] = useState(false);
  
  const historyRef = useRef<HTMLDivElement>(null);
  const templatesRef = useRef<HTMLDivElement>(null);

  const { data: tables } = useTables(database || "");

  // Load query history on mount
  useEffect(() => {
    const saved = localStorage.getItem(`sld_sql_query_${database}`);
    if (autoSave && saved) setQuery(saved);
    setHistory(loadHistory());
  }, [database, autoSave]);

  // Auto-save query
  useEffect(() => {
    if (autoSave && database) {
      localStorage.setItem(`sld_sql_query_${database}`, query);
    }
  }, [query, autoSave, database]);

  // Persist auto-save preference
  useEffect(() => {
    localStorage.setItem("sld_sql_autosave", String(autoSave));
  }, [autoSave]);

  // Close dropdowns when clicking outside
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        historyRef.current &&
        !historyRef.current.contains(e.target as Node)
      ) {
        setShowHistory(false);
      }
      if (
        templatesRef.current &&
        !templatesRef.current.contains(e.target as Node)
      ) {
        setShowTemplates(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleRunRef = useRef<() => void>(() => {});

  const { mutate: runExplain, isPending: explaining } = useMutation({
    mutationFn: () => explainQuery(database || "", query),
    onSuccess: (data) => {
      setExplanation(data);
      setShowExplanation(true);
    },
  });

  const { mutate: runQuery, isPending, error, reset: resetQuery } = useMutation({
    mutationFn: (vars: { database: string; query: string }) => {
      return executeQuery(vars.database, vars.query);
    },
    onSuccess: (data) => {
      setResult(data);
      // Add to history
      if (query.trim() && database) {
        const entry: HistoryEntry = {
          query: query.trim(),
          database,
          timestamp: Date.now(),
        };
        const newHistory = [
          entry,
          ...history.filter((h) => h.query !== entry.query),
        ];
        setHistory(newHistory);
        saveHistory(newHistory);
      }
    },
  });

  const handleRun = () => {
    if (query.trim() && database) {
      runQuery({ database, query });
    }
  };

  useEffect(() => {
    handleRunRef.current = handleRun;
  }, [handleRun]);

  const handleEditorMount = (editor: any, monaco: any) => {
    // Register SQL keywords autocomplete
    // Register SQL keywords and schema autocomplete
    monaco.languages.registerCompletionItemProvider("sql", {
      provideCompletionItems: (model: any, position: any) => {
        const suggestions: any[] = [];
        
        // Keywords
        const keywords = [
          "SELECT", "FROM", "WHERE", "INSERT", "INTO", "VALUES", "UPDATE", "SET", 
          "DELETE", "CREATE", "TABLE", "DROP", "ALTER", "INDEX", "JOIN", "LEFT", 
          "RIGHT", "INNER", "OUTER", "ON", "AND", "OR", "NOT", "NULL", "IS", 
          "LIKE", "IN", "BETWEEN", "ORDER", "BY", "ASC", "DESC", "LIMIT", "OFFSET", 
          "GROUP", "HAVING", "DISTINCT", "AS", "COUNT", "SUM", "AVG", "MAX", "MIN", 
          "SHOW", "TABLES", "DATABASES", "DESCRIBE", "EXPLAIN", "TRUNCATE"
        ];

        keywords.forEach(kw => {
          suggestions.push({
            label: kw,
            kind: monaco.languages.CompletionItemKind.Keyword,
            insertText: kw,
            range: {
              startLineNumber: position.lineNumber,
              endLineNumber: position.lineNumber,
              startColumn: model.getWordUntilPosition(position).startColumn,
              endColumn: position.column
            }
          });
        });

        // Tables
        if (tables) {
          tables.forEach((t: { name: string; engine?: string }) => {
            suggestions.push({
              label: t.name,
              kind: monaco.languages.CompletionItemKind.Struct,
              insertText: `\`${t.name}\``,
              detail: `Table (${t.engine})`,
              range: {
                startLineNumber: position.lineNumber,
                endLineNumber: position.lineNumber,
                startColumn: model.getWordUntilPosition(position).startColumn,
                endColumn: position.column
              }
            });
          });
        }

        return { suggestions };
      },
    });

    // Add Cmd/Ctrl+Enter to run query
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      handleRunRef.current();
    });
  };

  const copyToClipboard = async (text: string, type: string) => {
    await navigator.clipboard.writeText(text);
    setCopyFeedback(type);
    setTimeout(() => setCopyFeedback(null), 2000);
  };

  const handleCopyCSV = () => {
    if (result) {
      const csv = convertToCSV(result.columns, result.rows);
      copyToClipboard(csv, "csv");
    }
  };

  const handleCopyJSON = () => {
    if (result) {
      copyToClipboard(JSON.stringify(result.rows, null, 2), "json");
    }
  };

  const insertTemplate = (sql: string) => {
    setQuery(sql);
    setShowTemplates(false);
  };

  const insertFromHistory = (entry: HistoryEntry) => {
    setQuery(entry.query);
    setShowHistory(false);
  };

  const clearHistory = () => {
    setHistory([]);
    localStorage.removeItem(HISTORY_KEY);
  };

  const handleEditCell = (rowIdx: number, col: string, currentValue: string | number | boolean | null) => {
    // Check if we can identify the table
    const match = query.match(/FROM\s+`?([a-zA-Z0-9_]+)`?/i);
    if (!match) {
      alert("Cannot edit cell: Could not automatically determine the table name from your query. Please use a simple SELECT query.");
      return;
    }
    setEditingCell({ rowIdx, col, value: String(currentValue === null ? "" : currentValue) });
  };

  const handleSaveCell = async () => {
    if (!editingCell || !result || !database) return;
    
    const { rowIdx, col, value } = editingCell;
    const row = result.rows[rowIdx];
    const originalValue = row[col];
    
    if (String(originalValue) === value) {
      setEditingCell(null); // No change
      return;
    }

    const match = query.match(/FROM\s+`?([a-zA-Z0-9_]+)`?/i);
    const tableName = match ? match[1] : null;
    
    if (!tableName) {
      alert("Could not determine table name.");
      setEditingCell(null);
      return;
    }

    // Determine a row identifier. Prefer 'id', otherwise use the first column
    const pkCol = result.columns.includes("id") ? "id" : result.columns[0];
    const pkValue = row[pkCol];

    if (pkValue === undefined || pkValue === null) {
      alert("Could not determine a safe identifier (primary key) for this row.");
      setEditingCell(null);
      return;
    }

    const escapeValue = (val: string) => {
      if (val === "" && originalValue === null) return "NULL"; // rough guess
      const num = Number(val);
      if (!isNaN(num) && val.trim() !== "") return val;
      return `'${val.replace(/'/g, "''")}'`;
    };

    const updateQuery = `UPDATE \`${tableName}\` SET \`${col}\` = ${escapeValue(value)} WHERE \`${pkCol}\` = '${pkValue}' LIMIT 1`;

    try {
      await executeQuery(database, updateQuery);
      
      // Update local state to reflect change without re-running the main query
      const newResult = { ...result };
      newResult.rows = [...result.rows];
      newResult.rows[rowIdx] = { ...newResult.rows[rowIdx], [col]: value };
      setResult(newResult);
      
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      alert(`Update failed: ${message}`);
    } finally {
      setEditingCell(null);
    }
  };

  return (
    <div className="space-y-4 p-4">
      {/* Toolbar */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* Templates Dropdown */}
        <div className="relative" ref={templatesRef}>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setShowTemplates(!showTemplates)}
            className="gap-1"
          >
            <FileText size={14} />
            Templates
            <ChevronDown size={12} />
          </Button>
          {showTemplates && (
            <div className="absolute z-50 top-full left-0 mt-1 bg-[var(--card)] border border-[var(--border)] rounded-lg shadow-xl min-w-[250px] max-h-[300px] overflow-auto">
              {SQL_TEMPLATES.map((t, i) => (
                <button
                  key={i}
                  onClick={() => insertTemplate(t.sql)}
                  className="w-full text-left px-3 py-2 hover:bg-[var(--muted)]/50 text-sm border-b border-[var(--border)] last:border-0"
                >
                  <div className="font-medium">{t.label}</div>
                  <code className="text-xs text-[var(--muted-foreground)] truncate block">
                    {t.sql}
                  </code>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* History Dropdown */}
        <div className="relative" ref={historyRef}>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setShowHistory(!showHistory)}
            className="gap-1"
            disabled={history.length === 0}
          >
            <History size={14} />
            History ({history.length})
            <ChevronDown size={12} />
          </Button>
          {showHistory && history.length > 0 && (
            <div className="absolute z-50 top-full left-0 mt-1 bg-[var(--card)] border border-[var(--border)] rounded-lg shadow-xl min-w-[350px] max-h-[400px] overflow-auto">
              <div className="sticky top-0 bg-[var(--muted)]/80 backdrop-blur px-3 py-2 border-b border-[var(--border)] flex justify-between items-center">
                <span className="text-xs font-medium text-[var(--muted-foreground)]">
                  Query History
                </span>
                <button
                  onClick={clearHistory}
                  className="text-xs text-red-500 hover:underline"
                >
                  Clear All
                </button>
              </div>
              {history.map((h, i) => (
                <button
                  key={i}
                  onClick={() => insertFromHistory(h)}
                  className="w-full text-left px-3 py-2 hover:bg-[var(--muted)]/50 text-sm border-b border-[var(--border)] last:border-0"
                >
                  <code className="text-xs block font-mono text-[var(--foreground)] truncate">
                    {h.query}
                  </code>
                  <div className="text-[10px] text-[var(--muted-foreground)] mt-1 flex gap-2">
                    <span>{h.database}</span>
                    <span>•</span>
                    <span>{new Date(h.timestamp).toLocaleString()}</span>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        <Button
          variant={showBuilder ? "primary" : "secondary"}
          size="sm"
          onClick={() => setShowBuilder(!showBuilder)}
          className="gap-1"
        >
          <Database size={14} />
          Visual Builder
        </Button>

        <Button
          variant={autoSave ? "primary" : "secondary"}
          size="sm"
          onClick={() => setAutoSave(!autoSave)}
          className="gap-1"
          title={autoSave ? "Auto Save is ON" : "Auto Save is OFF"}
        >
          <Save size={14} className={autoSave ? "text-white" : "text-[var(--muted-foreground)]"} />
          <span className="hidden md:inline">Auto Save</span>
          <div className={`w-2 h-2 rounded-full ${autoSave ? "bg-green-400" : "bg-gray-400"}`} />
        </Button>

        <Button
          variant="secondary"
          size="sm"
          onClick={() => runExplain()}
          disabled={!query || explaining}
          className="gap-1 border-blue-500/30 text-blue-400 hover:bg-blue-500/10"
        >
          <AlertCircle size={14} />
          {explaining ? "Analyzing..." : "Explain AI"}
        </Button>

        <Button
          size="sm"
          variant="secondary"
          onClick={() => setQuery("")}
          disabled={!query}
        >
          <Eraser size={14} />
        </Button>
        <Button
          size="sm"
          onClick={handleRun}
          disabled={!query.trim() || !database || isPending}
          loading={isPending}
          className="gap-2"
        >
          <Play size={14} />
          Run
        </Button>
      </div>

      {showBuilder && database && (
        <QueryBuilder 
          database={database}
          onGenerate={(sql) => {
            setQuery(sql);
          }}
          onClose={() => setShowBuilder(false)}
        />
      )}

      {/* Editor & Explanation Panel */}
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">
        <div className={showExplanation ? "lg:col-span-3" : "lg:col-span-4"}>
          <div className="border border-[var(--border)] rounded-lg overflow-hidden bg-[#1e1e1e]">
            <Editor
              height="300px"
              defaultLanguage="sql"
              theme="vs-dark"
              value={query}
              onChange={(val) => setQuery(val || "")}
              options={{
                minimap: { enabled: false },
                fontSize: 14,
                fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
                scrollBeyondLastLine: false,
                automaticLayout: true,
                padding: { top: 16, bottom: 16 },
              }}
              onMount={handleEditorMount}
            />
          </div>
        </div>

        {showExplanation && explanation && (
          <div className="lg:col-span-1 border border-blue-500/30 rounded-lg bg-blue-500/5 p-4 space-y-4 animate-in fade-in slide-in-from-right-4 duration-300">
            <div className="flex justify-between items-center border-b border-blue-500/20 pb-2">
              <h3 className="text-sm font-bold text-blue-400 flex items-center gap-2">
                <AlertCircle size={14} />
                Query Analysis
              </h3>
              <button 
                onClick={() => setShowExplanation(false)}
                className="text-blue-500 hover:text-blue-400 text-xs"
              >
                Close
              </button>
            </div>
            
            <div className="space-y-3">
              <div>
                <div className="text-[10px] uppercase font-bold text-blue-500/70 mb-1">Complexity</div>
                <div className={`text-xs px-2 py-0.5 rounded inline-block font-bold ${
                  explanation.complexity === 'Simple' ? 'bg-green-500/20 text-green-400' :
                  explanation.complexity === 'Moderate' ? 'bg-yellow-500/20 text-yellow-400' :
                  'bg-red-500/20 text-red-400'
                }`}>
                  {explanation.complexity}
                </div>
              </div>

              <div>
                <div className="text-[10px] uppercase font-bold text-blue-500/70 mb-1">Analysis</div>
                <ul className="text-xs space-y-1.5 list-disc list-inside text-blue-200/80">
                  {explanation.analysis.length > 0 ? (
                    explanation.analysis.map((a, i) => <li key={i}>{a}</li>)
                  ) : (
                    <li>Query appears to be well-optimized.</li>
                  )}
                </ul>
              </div>

              <div>
                <div className="text-[10px] uppercase font-bold text-blue-500/70 mb-1">Recommendations</div>
                <ul className="text-xs space-y-1.5 list-disc list-inside text-blue-200/80">
                   {explanation.recommendations.map((r, i) => (
                    <li key={i} className="text-blue-300 font-medium">{r}</li>
                  ))}
                  {explanation.recommendations.length === 0 && (
                    <li>No changes recommended.</li>
                  )}
                </ul>
              </div>

              <div className="pt-2 border-t border-blue-500/20">
                <div className="text-[10px] text-blue-500/50">
                  Estimated rows scanned: <span className="text-blue-400 font-mono">{explanation.estimated_rows.toLocaleString()}</span>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Status Bar */}
      <div className="text-xs text-[var(--muted-foreground)] flex justify-between px-1">
        <span>
          Database:{" "}
          <span className="font-mono font-medium text-[var(--foreground)]">
            {database || "(None)"}
          </span>
        </span>
        <span className="flex items-center gap-2">
          <Clock size={12} />
          Cmd/Ctrl + Enter to run
        </span>
      </div>

      {/* Results */}
      <div className="mt-4">
        {error ? (
          <Card className="bg-red-500/5 border-red-500/20 p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-start gap-3 text-red-500">
                <AlertCircle size={18} className="mt-0.5 flex-shrink-0" />
                <div className="font-mono text-sm whitespace-pre-wrap break-words">
                  {(error as Error).message}
                </div>
              </div>
              <Button size="sm" variant="ghost" onClick={() => resetQuery()}>
                <Eraser size={14} />
              </Button>
            </div>
          </Card>
        ) : result ? (
          <div className="border border-[var(--border)] rounded-lg overflow-hidden bg-[var(--card)] shadow-sm">
            {/* Results Header */}
            <div className="p-2 border-b border-[var(--border)] bg-[var(--muted)]/30 text-xs text-[var(--muted-foreground)] font-mono flex items-center gap-4 flex-wrap">
              <span className="flex items-center gap-1">
                {result.row_count} rows
              </span>
              {result.affected_rows !== undefined &&
                result.affected_rows > 0 && (
                  <span className="text-green-500">
                    {result.affected_rows} affected
                  </span>
                )}
              {result.execution_time_ms !== undefined && (
                <span className="flex items-center gap-1">
                  <Clock size={12} />
                  {formatTime(result.execution_time_ms)}
                </span>
              )}
              <div className="flex-1" />
              {result.rows.length > 0 && (
                <div className="flex gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleCopyCSV}
                    className="h-6 px-2 text-xs gap-1"
                  >
                    {copyFeedback === "csv" ? (
                      <Check size={12} className="text-green-500" />
                    ) : (
                      <Copy size={12} />
                    )}
                    CSV
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleCopyJSON}
                    className="h-6 px-2 text-xs gap-1"
                  >
                    {copyFeedback === "json" ? (
                      <Check size={12} className="text-green-500" />
                    ) : (
                      <Download size={12} />
                    )}
                    JSON
                  </Button>
                </div>
              )}
            </div>

            {/* Results Table */}
            <div className="overflow-x-auto">
              <table className="w-full text-sm font-mono whitespace-nowrap">
                <thead className="bg-[var(--muted)]/50 sticky top-0">
                  <tr>
                    {result.columns.map((col) => (
                      <th
                        key={col}
                        className="px-4 py-2 text-left font-medium border-b border-[var(--border)] text-[var(--muted-foreground)]"
                      >
                        {col}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {result.rows.map((row, i) => (
                    <tr
                      key={i}
                      className="hover:bg-[var(--card-hover)] border-b border-[var(--border)] last:border-0"
                    >
                      {result.columns.map((col, j) => (
                        <td
                          key={j}
                          className="px-4 py-1.5 border-r border-[var(--border)] last:border-0 text-[var(--foreground)] cursor-pointer group hover:bg-[var(--primary)]/10"
                          onDoubleClick={() => handleEditCell(i, col, row[col])}
                        >
                          {editingCell?.rowIdx === i && editingCell.col === col ? (
                            <input
                              type="text"
                              autoFocus
                              className="w-full bg-[var(--background)] border border-[var(--primary)] rounded px-1 outline-none text-sm text-[var(--foreground)]"
                              value={editingCell.value}
                              onChange={(e) => setEditingCell({ ...editingCell, value: e.target.value })}
                              onBlur={handleSaveCell}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") handleSaveCell();
                                if (e.key === "Escape") setEditingCell(null);
                              }}
                            />
                          ) : row[col] === null ? (
                            <span className="text-[var(--muted-foreground)] italic">
                              NULL
                            </span>
                          ) : (
                            String(row[col])
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                  {result.rows.length === 0 && (
                    <tr>
                      <td
                        colSpan={result.columns.length || 1}
                        className="px-4 py-8 text-center text-[var(--muted-foreground)]"
                      >
                        {result.columns.length > 0
                          ? "Query returned no rows."
                          : "Query executed successfully."}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-center border border-dashed border-[var(--border)] rounded-lg text-[var(--muted-foreground)] bg-[var(--card)]/50 py-16">
            <div className="text-center">
              <code className="block mb-2 text-xs opacity-50">READY</code>
              <p className="text-sm">Execute a query to see results here</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
