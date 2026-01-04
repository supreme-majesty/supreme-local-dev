import { useState, useEffect, useRef } from "react";
import { useMutation } from "@tanstack/react-query";
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
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";

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
  const escape = (val: any) => {
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
  const [result, setResult] = useState<QueryResult | null>(null);
  const [history, setHistory] = useState<HistoryEntry[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const [showTemplates, setShowTemplates] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);
  const historyRef = useRef<HTMLDivElement>(null);
  const templatesRef = useRef<HTMLDivElement>(null);

  // Load history on mount
  useEffect(() => {
    setHistory(loadHistory());
  }, []);

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

  const mutation = useMutation({
    mutationFn: () => {
      if (!database) throw new Error("No database selected");
      return executeQuery(database, query);
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
      mutation.mutate();
    }
  };

  const handleEditorMount = (editor: any, monaco: any) => {
    // Register SQL keywords autocomplete
    monaco.languages.registerCompletionItemProvider("sql", {
      provideCompletionItems: () => {
        const keywords = [
          "SELECT",
          "FROM",
          "WHERE",
          "INSERT",
          "INTO",
          "VALUES",
          "UPDATE",
          "SET",
          "DELETE",
          "CREATE",
          "TABLE",
          "DROP",
          "ALTER",
          "INDEX",
          "JOIN",
          "LEFT",
          "RIGHT",
          "INNER",
          "OUTER",
          "ON",
          "AND",
          "OR",
          "NOT",
          "NULL",
          "IS",
          "LIKE",
          "IN",
          "BETWEEN",
          "ORDER",
          "BY",
          "ASC",
          "DESC",
          "LIMIT",
          "OFFSET",
          "GROUP",
          "HAVING",
          "DISTINCT",
          "AS",
          "COUNT",
          "SUM",
          "AVG",
          "MAX",
          "MIN",
          "SHOW",
          "TABLES",
          "DATABASES",
          "DESCRIBE",
          "EXPLAIN",
          "TRUNCATE",
        ];
        return {
          suggestions: keywords.map((kw) => ({
            label: kw,
            kind: monaco.languages.CompletionItemKind.Keyword,
            insertText: kw,
          })),
        };
      },
    });

    // Add Cmd/Ctrl+Enter to run query
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
      handleRun();
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

  return (
    <div className="space-y-4 h-full flex flex-col p-4">
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

        <div className="flex-1" />

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
          disabled={!query.trim() || !database || mutation.isPending}
          loading={mutation.isPending}
          className="gap-2"
        >
          <Play size={14} />
          Run
        </Button>
      </div>

      {/* Editor */}
      <div className="border border-[var(--border)] rounded-lg overflow-hidden bg-[var(--card)]">
        <Editor
          height="200px"
          language="sql"
          theme="vs-dark"
          value={query}
          onChange={(val) => setQuery(val || "")}
          onMount={handleEditorMount}
          options={{
            minimap: { enabled: false },
            fontSize: 14,
            lineNumbers: "on",
            scrollBeyondLastLine: false,
            wordWrap: "on",
            padding: { top: 12, bottom: 12 },
            automaticLayout: true,
            tabSize: 2,
            suggestOnTriggerCharacters: true,
            quickSuggestions: true,
          }}
        />
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
      <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
        {mutation.error ? (
          <Card className="bg-red-500/5 border-red-500/20 p-4">
            <div className="flex items-start gap-3 text-red-500">
              <AlertCircle size={18} className="mt-0.5 flex-shrink-0" />
              <div className="font-mono text-sm whitespace-pre-wrap break-words">
                {mutation.error.message}
              </div>
            </div>
          </Card>
        ) : result ? (
          <div className="flex-1 flex flex-col min-h-0 border border-[var(--border)] rounded-lg overflow-hidden bg-[var(--card)]">
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
            <div className="flex-1 overflow-auto">
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
                          className="px-4 py-1.5 border-r border-[var(--border)] last:border-0 text-[var(--foreground)]"
                        >
                          {row[col] === null ? (
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
          <div className="flex-1 flex items-center justify-center border border-dashed border-[var(--border)] rounded-lg text-[var(--muted-foreground)] bg-[var(--card)]/50">
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
