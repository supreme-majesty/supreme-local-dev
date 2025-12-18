import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Play, Eraser, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";

interface SQLConsoleProps {
  database: string | null;
}

interface QueryResult {
  columns: string[];
  rows: Record<string, any>[];
  row_count: number;
}

async function executeQuery(
  database: string,
  query: string
): Promise<QueryResult> {
  const res = await fetch("/api/db/query", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ database, query }),
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || "Query failed");
  }

  return res.json();
}

export function SQLConsole({ database }: SQLConsoleProps) {
  const [query, setQuery] = useState("");
  const [result, setResult] = useState<QueryResult | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      if (!database) throw new Error("No database selected");
      return executeQuery(database, query);
    },
    onSuccess: (data) => {
      setResult(data);
    },
    onError: (err: Error) => {
      // Error handling is done via mutation.error state in render
      console.error(err);
    },
  });

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      if (query.trim() && database) {
        mutation.mutate();
      }
    }
  };

  return (
    <div className="space-y-4 h-full flex flex-col p-4">
      {/* Editor & Actions */}
      <div className="flex flex-col gap-3">
        <div className="relative">
          <textarea
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="SELECT * FROM users..."
            className="w-full h-40 font-mono text-sm p-4 bg-[var(--card)] border border-[var(--border)] rounded-lg focus:outline-none focus:ring-2 focus:ring-[var(--primary)]/50 resize-y"
          />
          <div className="absolute bottom-3 right-3 flex gap-2">
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
              onClick={() => mutation.mutate()}
              disabled={!query.trim() || !database || mutation.isPending}
              loading={mutation.isPending}
            >
              <Play size={14} className="mr-2" />
              Run
            </Button>
          </div>
        </div>
        <div className="text-xs text-[var(--muted-foreground)] flex justify-between px-1">
          <span>
            Target Database:{" "}
            <span className="font-mono font-medium text-[var(--foreground)]">
              {database || "(None Selected)"}
            </span>
          </span>
          <span>Cmd/Ctrl + Enter to run</span>
        </div>
      </div>

      {/* Results */}
      <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
        {mutation.error ? (
          <Card className="bg-red-500/5 border-red-500/20 p-4">
            <div className="flex items-start gap-3 text-red-500">
              <AlertCircle size={18} className="mt-0.5" />
              <div className="font-mono text-sm whitespace-pre-wrap break-words">
                {mutation.error.message}
              </div>
            </div>
          </Card>
        ) : result ? (
          <div className="flex-1 flex flex-col min-h-0 border border-[var(--border)] rounded-lg overflow-hidden bg-[var(--card)]">
            <div className="p-2 border-b border-[var(--border)] bg-[var(--muted)]/30 text-xs text-[var(--muted-foreground)] font-mono flex gap-4">
              <span>{result.row_count} rows</span>
              <span>{new Date().toLocaleTimeString()}</span>
            </div>
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
                        colSpan={result.columns.length}
                        className="px-4 py-8 text-center text-[var(--muted-foreground)]"
                      >
                        Query returned no rows.
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
