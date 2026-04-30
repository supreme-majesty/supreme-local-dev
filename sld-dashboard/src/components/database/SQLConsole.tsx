/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useEffect, useRef, useCallback } from "react";
import { useMutation } from "@tanstack/react-query";
import { useTables, useSnippets, useSaveSnippetMutation } from "@/hooks/use-database";
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
  Plus,
  X,
  BookOpen,
  BarChart3,
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { QueryBuilder } from "./QueryBuilder";
import { ChartBuilder } from "./ChartBuilder";

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

interface QueryTab {
  id: string;
  title: string;
  query: string;
  result: QueryResult | null;
  isExecuting: boolean;
  error: string | null;
  txId?: string;
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
  const res = await fetch("/api/db/query/analyze", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, query }),
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Explain failed");
  }

  const plan = await res.json();
  
  // Intelligence Logic: Parse plan rows to generate a human explanation
  const analysis: string[] = [];
  const recommendations: string[] = [];
  let estimatedRows = 0;
  let complexity = "Simple";

  if (plan.rows && plan.rows.length > 0) {
    (plan.rows as any[]).forEach((row: any) => {
      estimatedRows += parseInt(row.rows || "0");
      
      const type = row.type || "";
      const extra = row.Extra || "";

      if (type === "ALL") {
        analysis.push(`Full table scan on \`${row.table}\`.`);
        complexity = "High";
        recommendations.push(`Add an index to columns used in WHERE/JOIN clauses for table \`${row.table}\`.`);
      }
      
      if (extra.includes("Using temporary") || extra.includes("Using filesort")) {
        analysis.push(`Query uses temporary tables or disk-based sorting for \`${row.table}\`.`);
        complexity = complexity === "High" ? "Critical" : "Moderate";
        recommendations.push(`Optimize ORDER BY or GROUP BY clauses to use existing indexes.`);
      }

      if (type === "index") {
        analysis.push(`Full index scan on \`${row.table}\`. Better than table scan but still potentially slow.`);
      }
    });
  }

  if (analysis.length === 0) {
     analysis.push("Query is using efficient index lookups.");
  }

  return {
    analysis,
    recommendations,
    estimated_rows: estimatedRows,
    complexity
  };
}

async function executeQuery(
  database: string,
  query: string,
  txId?: string
): Promise<QueryResult> {
  const startTime = performance.now();
  const res = await fetch("/api/db/query", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, query, tx_id: txId }),
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

function convertToCSV(columns: string[], rows: any[]): string {
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
  const [tabs, setTabs] = useState<QueryTab[]>(() => {
    const saved = localStorage.getItem(`sld_sql_tabs_${database}`);
    if (saved) return JSON.parse(saved);
    return [
      {
        id: "1",
        title: "Query 1",
        query: "",
        result: null,
        isExecuting: false,
        error: null,
      },
    ];
  });
  const [activeTabId, setActiveTabId] = useState<string>("1");
  const activeTabIdRef = useRef(activeTabId);
  useEffect(() => {
    activeTabIdRef.current = activeTabId;
  }, [activeTabId]);

  const activeTab = tabs.find((t) => t.id === activeTabId) || tabs[0];

  const [autoSave, setAutoSave] = useState(() => {
    return localStorage.getItem("sld_sql_autosave") === "true";
  });
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const [showTemplates, setShowTemplates] = useState(false);
  const [showBuilder, setShowBuilder] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const [editingCell, setEditingCell] = useState<{
    rowIdx: number;
    col: string;
    value: string;
  } | null>(null);

  const [explanation, setExplanation] = useState<QueryExplanation | null>(null);
  const [showExplanation, setShowExplanation] = useState(false);
  
  const historyRef = useRef<HTMLDivElement>(null);
  const templatesRef = useRef<HTMLDivElement>(null);

  const { data: tables } = useTables(database || "");
  const { data: snippets = [] } = useSnippets();
  const saveSnippetMutation = useSaveSnippetMutation();
  const [isSavingSnippet, setIsSavingSnippet] = useState(false);
  const [snippetLabel, setSnippetLabel] = useState("");
  const [viewMode, setViewMode] = useState<'table' | 'chart'>('table');

  // Save tabs on change
  useEffect(() => {
    if (database) {
      localStorage.setItem(`sld_sql_tabs_${database}`, JSON.stringify(tabs));
    }
  }, [tabs, database]);

  // Persist auto-save preference
  useEffect(() => {
    localStorage.setItem("sld_sql_autosave", String(autoSave));
  }, [autoSave]);

  const addTab = () => {
    const id = Date.now().toString();
    const newTab: QueryTab = {
      id,
      title: `Query ${tabs.length + 1}`,
      query: "",
      result: null,
      isExecuting: false,
      error: null,
    };
    setTabs([...tabs, newTab]);
    setActiveTabId(id);
  };

  const closeTab = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (tabs.length === 1) return;
    const newTabs = tabs.filter((t) => t.id !== id);
    setTabs(newTabs);
    if (activeTabId === id) {
      setActiveTabId(newTabs[newTabs.length - 1].id);
    }
  };

  const updateActiveTab = (updates: Partial<QueryTab>) => {
    setTabs((prev) =>
      prev.map((t) => (t.id === activeTabIdRef.current ? { ...t, ...updates } : t))
    );
  };

  const updateTab = (id: string, updates: Partial<QueryTab>) => {
    setTabs((prev) => prev.map((t) => (t.id === id ? { ...t, ...updates } : t)));
  };

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
    mutationFn: ({ query }: { query: string }) =>
      explainQuery(database || "", query),
    onSuccess: (data) => {
      setExplanation(data);
      setShowExplanation(true);
    },
  });

  const { mutate: runQuery, isPending, error: queryError, reset: resetQuery } = useMutation({
    mutationFn: async (vars: { database: string; query: string; txId?: string; tabId: string }) => {
      const data = await executeQuery(vars.database, vars.query, vars.txId);
      return { data, tabId: vars.tabId };
    },
    onSuccess: ({ data, tabId }) => {
      updateTab(tabId, { result: data, error: null });
      // Add to history
      if (activeTab.query.trim() && database) {
        const entry: HistoryEntry = {
          query: activeTab.query.trim(),
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
    onError: (err, vars) => {
      if (err instanceof Error && err.message.includes("not found or expired")) {
        updateTab(vars.tabId, { txId: undefined });
      }
    }
  });
  const handleRun = useCallback(() => {
    if (activeTab.query.trim() && database) {
      runQuery({ database, query: activeTab.query, txId: activeTab.txId, tabId: activeTab.id });
    }
  }, [activeTab.query, activeTab.txId, activeTab.id, database, runQuery]);

  useEffect(() => {
    handleRunRef.current = handleRun;
  }, [handleRun]);

  const handleBeginTx = async () => {
    if (!database) return;
    try {
      const res = await fetch("/api/db/transaction", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ database, operation: "begin" }),
      });
      const data = await res.json();
      if (data.success) {
        updateActiveTab({ txId: data.tx_id });
      }
    } catch (e) {
      console.error("Failed to begin transaction", e);
    }
  };

  const handleCommitTx = async () => {
    if (!activeTab.txId) return;
    try {
      const res = await fetch("/api/db/transaction", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ operation: "commit", tx_id: activeTab.txId }),
      });
      const data = await res.json();
      if (data.success) {
        updateActiveTab({ txId: undefined });
      } else if (data.error && data.error.includes("not found")) {
        updateActiveTab({ txId: undefined });
      }
    } catch (e: unknown) {
      const error = e as Error;
      console.error("Failed to commit transaction", error);
      if (error?.message?.includes("not found")) {
        updateActiveTab({ txId: undefined });
      }
    }
  };

  const handleRollbackTx = async () => {
    if (!activeTab.txId) return;
    try {
      const res = await fetch("/api/db/transaction", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ operation: "rollback", tx_id: activeTab.txId }),
      });
      const data = await res.json();
      if (data.success) {
        updateActiveTab({ txId: undefined });
      } else if (data.error && data.error.includes("not found")) {
        updateActiveTab({ txId: undefined });
      }
    } catch (e: unknown) {
      const error = e as Error;
      console.error("Failed to rollback transaction", error);
      if (error?.message?.includes("not found")) {
        updateActiveTab({ txId: undefined });
      }
    }
  };

  const handleEditorMount = (editor: { addCommand: (key: number, cb: () => void) => void }, monaco: { languages: { registerCompletionItemProvider: (lang: string, provider: unknown) => void; CompletionItemKind: Record<string, number> }; KeyMod: { CtrlCmd: number }; KeyCode: { Enter: number } }) => {
    // Register SQL keywords autocomplete
    // Register SQL keywords and schema autocomplete
    monaco.languages.registerCompletionItemProvider("sql", {
      provideCompletionItems: (model: { getWordUntilPosition: (pos: { lineNumber: number; column: number }) => { startColumn: number } }, position: { lineNumber: number; column: number }) => {
        const suggestions: { label: string; kind: number; insertText: string; range: { startLineNumber: number; endLineNumber: number; startColumn: number; endColumn: number }; detail?: string }[] = [];
        
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
    if (activeTab.result) {
      const csv = convertToCSV(activeTab.result.columns, activeTab.result.rows);
      copyToClipboard(csv, "csv");
    }
  };

  const handleCopyJSON = () => {
    if (activeTab.result) {
      copyToClipboard(JSON.stringify(activeTab.result.rows, null, 2), "json");
    }
  };

  const insertTemplate = (sql: string) => {
    updateActiveTab({ query: sql });
    setShowTemplates(false);
  };

  const insertFromHistory = (entry: HistoryEntry) => {
    updateActiveTab({ query: entry.query });
    setShowHistory(false);
  };

  const clearHistory = () => {
    setHistory([]);
    localStorage.removeItem(HISTORY_KEY);
  };

  const handleEditCell = (rowIdx: number, col: string, currentValue: string | number | boolean | null) => {
    // Check if we can identify the table
    const match = activeTab.query.match(/FROM\s+`?([a-zA-Z0-9_]+)`?/i);
    if (!match) {
      alert("Cannot edit cell: Could not automatically determine the table name from your query. Please use a simple SELECT query.");
      return;
    }
    setEditingCell({ rowIdx, col, value: String(currentValue === null ? "" : currentValue) });
  };

  const handleSaveCell = async () => {
    if (!editingCell || !activeTab.result || !database) return;
    
    const { rowIdx, col, value } = editingCell;
    const row = activeTab.result.rows[rowIdx];
    const originalValue = row[col];
    
    if (String(originalValue) === value) {
      setEditingCell(null); // No change
      return;
    }

    const match = activeTab.query.match(/FROM\s+`?([a-zA-Z0-9_]+)`?/i);
    const tableName = match ? match[1] : null;
    
    if (!tableName) {
      alert("Could not determine table name.");
      setEditingCell(null);
      return;
    }

    // Determine a row identifier. Prefer 'id', otherwise use the first column
    const pkCol = activeTab.result.columns.includes("id") ? "id" : activeTab.result.columns[0];
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
      const newResult = { ...activeTab.result };
      newResult.rows = [...activeTab.result.rows];
      newResult.rows[rowIdx] = { ...newResult.rows[rowIdx], [col]: value };
      updateActiveTab({ result: newResult });
      
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      alert(`Update failed: ${message}`);
    } finally {
      setEditingCell(null);
    }
  };

  const handleSaveSnippet = async () => {
    if (!activeTab.query || !snippetLabel) return;
    await saveSnippetMutation.mutateAsync({
      id: Date.now().toString(),
      label: snippetLabel,
      sql: activeTab.query,
      database: database || "",
      tags: [],
      created_at: new Date()
    });
    setIsSavingSnippet(false);
    setSnippetLabel("");
  };

  return (
    <div className="flex h-full gap-4 p-4 overflow-hidden">
      {/* Snippets Sidebar */}
      <div className="w-64 flex flex-col gap-4">
        <Card className="flex-1 border-[var(--border)] overflow-hidden flex flex-col">
          <CardHeader className="py-3 border-b border-[var(--border)] bg-[var(--muted)]/20">
            <CardTitle className="text-sm font-bold flex items-center gap-2">
              <BookOpen size={16} className="text-blue-500" /> Query Library
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0 flex-1 overflow-y-auto">
            <div className="divide-y divide-[var(--border)]">
              {snippets.length === 0 ? (
                <div className="p-8 text-center text-xs text-[var(--muted-foreground)] italic">
                  Your snippet library is empty
                </div>
              ) : (
                (snippets as any[]).map((s: { id: string; label: string; sql: string }) => (
                  <button
                    key={s.id}
                    onClick={() => updateActiveTab({ query: s.sql })}
                    className="w-full text-left p-3 hover:bg-[var(--muted)]/50 transition-all group"
                  >
                    <div className="font-bold text-xs truncate text-[var(--foreground)]">{s.label}</div>
                    <code className="text-[10px] text-[var(--muted-foreground)] truncate block mt-1">{s.sql}</code>
                  </button>
                ))
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="flex-1 flex flex-col gap-4 overflow-hidden">
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
          onClick={() => runExplain({ query: activeTab.query })}
          disabled={!activeTab.query || explaining}
          className="gap-1 border-blue-500/30 text-blue-400 hover:bg-blue-500/10"
        >
          <AlertCircle size={14} />
          {explaining ? "Analyzing..." : "Explain AI"}
        </Button>

        <div className="h-4 w-px bg-[var(--border)] mx-1" />

        {/* Transaction Controls */}
        {!activeTab.txId ? (
          <Button
            variant="secondary"
            size="sm"
            onClick={handleBeginTx}
            disabled={!database}
            className="gap-1 border-yellow-500/30 text-yellow-500 hover:bg-yellow-500/10"
          >
            <Clock size={14} />
            Begin Tx
          </Button>
        ) : (
          <div className="flex items-center gap-1 bg-yellow-500/10 border border-yellow-500/30 rounded px-1 py-0.5">
            <span className="text-[10px] text-yellow-500 font-bold px-2 uppercase animate-pulse">TX Active</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleCommitTx}
              className="h-7 text-green-500 hover:text-green-600 hover:bg-green-500/20 px-2 text-xs gap-1"
            >
              <Check size={12} /> Commit
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleRollbackTx}
              className="h-7 text-red-500 hover:text-red-600 hover:bg-red-500/20 px-2 text-xs gap-1"
            >
              <X size={12} /> Rollback
            </Button>
          </div>
        )}

        <Button
          size="sm"
          variant="secondary"
          onClick={() => updateActiveTab({ query: "" })}
          disabled={!activeTab.query}
        >
          <Eraser size={14} />
        </Button>
        <Button
          size="sm"
          onClick={handleRun}
          disabled={!activeTab.query.trim() || !database || isPending}
          loading={isPending}
          className="gap-2"
        >
          <Play size={14} />
          Run
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => setIsSavingSnippet(true)}
          disabled={!activeTab.query}
          className="gap-1 border-blue-500/30 text-blue-400 hover:bg-blue-500/10"
        >
          <Save size={14} />
          Save Snippet
        </Button>
      </div>

      {isSavingSnippet && (
        <Card className="p-4 border-blue-500/30 bg-blue-500/5 animate-in slide-in-from-top-4">
          <div className="flex items-center gap-4">
            <Input 
              placeholder="Snippet Label (e.g. Fetch Active Users)" 
              value={snippetLabel}
              onChange={(e) => setSnippetLabel(e.target.value)}
              className="flex-1 h-9"
              autoFocus
            />
            <Button size="sm" onClick={handleSaveSnippet} disabled={!snippetLabel}>Save</Button>
            <Button size="sm" variant="ghost" onClick={() => setIsSavingSnippet(false)}>Cancel</Button>
          </div>
        </Card>
      )}

      {showBuilder && database && (
        <QueryBuilder 
          database={database} 
          onGenerate={(q) => updateActiveTab({ query: q })} 
          onClose={() => setShowBuilder(false)} 
        />
      )}

      {/* Editor & Explanation Panel */}
      {/* Tab Bar */}
      <div className="flex items-center gap-1 overflow-x-auto pb-2 scrollbar-hide border-b border-[var(--border)] mb-2">
        {tabs.map((tab) => (
          <div
            key={tab.id}
            onClick={() => setActiveTabId(tab.id)}
            className={`flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-t-lg cursor-pointer border-t border-l border-r transition-all min-w-[120px] max-w-[200px] ${
              activeTabId === tab.id
                ? "bg-[var(--card)] border-[var(--border)] text-[var(--primary)] -mb-px shadow-[0_-2px_10px_rgba(var(--primary-rgb),0.1)]"
                : "bg-[var(--muted)]/30 border-transparent text-[var(--muted-foreground)] hover:bg-[var(--muted)]/50"
            }`}
          >
            <span className="truncate flex-1 font-mono text-xs">{tab.title}</span>
            {tabs.length > 1 && (
              <button
                onClick={(e) => closeTab(tab.id, e)}
                className="p-1 hover:bg-[var(--muted)] rounded-full transition-colors"
              >
                <X size={10} />
              </button>
            )}
          </div>
        ))}
        <button
          onClick={addTab}
          className="p-2 hover:bg-[var(--muted)]/50 text-[var(--muted-foreground)] rounded-lg transition-colors flex items-center justify-center ml-2"
          title="Add New Query Tab"
        >
          <Plus size={16} />
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-4">
        <div className={showExplanation ? "lg:col-span-3" : "lg:col-span-4"}>
          <div className="border border-[var(--border)] rounded-lg overflow-hidden bg-[#1e1e1e]">
            <Editor
              height="300px"
              defaultLanguage="sql"
              theme="vs-dark"
              value={activeTab.query}
              onChange={(val) => updateActiveTab({ query: val || "" })}
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
        {queryError ? (
          <Card className="bg-red-500/5 border-red-500/20 p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-start gap-3 text-red-500">
                <AlertCircle size={18} className="mt-0.5 flex-shrink-0" />
                <div className="font-mono text-sm whitespace-pre-wrap break-words">
                  {(queryError as Error).message}
                </div>
              </div>
              <Button size="sm" variant="ghost" onClick={() => resetQuery()}>
                <Eraser size={14} />
              </Button>
            </div>
          </Card>
        ) : activeTab.result ? (() => {
          const currentResult = activeTab.result;
          return (
            <div className="border border-[var(--border)] rounded-lg overflow-hidden bg-[var(--card)] shadow-sm">
              {/* Results Header */}
              <div className="p-2 border-b border-[var(--border)] bg-[var(--muted)]/30 text-xs text-[var(--muted-foreground)] font-mono flex items-center gap-4 flex-wrap">
                <span className="flex items-center gap-1">
                  {currentResult.row_count} rows
                </span>
                {currentResult.affected_rows !== undefined &&
                  currentResult.affected_rows > 0 && (
                    <span className="text-green-500">
                      {currentResult.affected_rows} affected
                    </span>
                  )}
                {currentResult.execution_time_ms !== undefined && (
                  <span className="flex items-center gap-1">
                    <Clock size={12} />
                    {formatTime(currentResult.execution_time_ms)}
                  </span>
                )}
                <div className="flex-1" />
                {currentResult.rows.length > 0 && (
                  <div className="flex gap-2">
                    <Button
                      variant={viewMode === 'table' ? 'secondary' : 'ghost'}
                      size="sm"
                      onClick={() => setViewMode('table')}
                      className="h-6 px-2 text-xs gap-1"
                    >
                      <Database size={12} />
                      Table
                    </Button>
                    <Button
                      variant={viewMode === 'chart' ? 'secondary' : 'ghost'}
                      size="sm"
                      onClick={() => setViewMode('chart')}
                      className="h-6 px-2 text-xs gap-1"
                    >
                      <BarChart3 size={12} />
                      Chart
                    </Button>
                    <div className="w-px h-4 bg-[var(--border)] mx-1 self-center" />
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

              {viewMode === 'chart' ? (
                <div className="p-4 border-t border-[var(--border)] min-h-[400px]">
                  <ChartBuilder data={currentResult.rows} columns={currentResult.columns} />
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm font-mono whitespace-nowrap">
                  <thead className="bg-[var(--muted)]/50 sticky top-0">
                    <tr>
                      {(currentResult.columns || []).map((col) => (
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
                    {(currentResult.rows || []).map((row, i) => (
                      <tr
                        key={i}
                        className="hover:bg-[var(--card-hover)] border-b border-[var(--border)] last:border-0"
                      >
                        {(currentResult.columns || []).map((col, j) => (
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
                    {currentResult.rows.length === 0 && (
                      <tr>
                        <td
                          colSpan={currentResult.columns.length || 1}
                          className="px-4 py-8 text-center text-[var(--muted-foreground)] italic"
                        >
                          {currentResult.columns.length > 0
                            ? "No rows returned"
                            : "Query executed successfully"}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
              )}
            </div>
          );
        })() : (
          <div className="flex items-center justify-center border border-dashed border-[var(--border)] rounded-lg text-[var(--muted-foreground)] bg-[var(--card)]/50 py-16">
            <div className="text-center">
              <code className="block mb-2 text-xs opacity-50">READY</code>
              <p className="text-sm">Execute a query to see results here</p>
            </div>
          </div>
        )}
      </div>
    </div>
  </div>
);
}
