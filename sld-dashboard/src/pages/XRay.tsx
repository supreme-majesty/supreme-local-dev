import { useEffect, useState, useRef } from "react";
import {
  Zap,
  Search,
  Trash2,
  Play,
  Pause,
  ChevronRight,
  Activity,
  ArrowUpRight,
  Clock,
  Database,
  Globe,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface XRayLog {
  id: string;
  time_iso: string;
  method: string;
  host: string;
  uri: string;
  status: number;
  latency: string;
  body_bytes: number;
  agent: string;
}

interface DBQueryLog {
  id: string;
  event_time: string;
  user_host: string;
  thread_id: number;
  server_id: number;
  command: string;
  argument: string;
}

export default function XRay() {
  const [mode, setMode] = useState<"http" | "database">("http");
  const [logs, setLogs] = useState<XRayLog[]>([]);
  const [dbLogs, setDbLogs] = useState<DBQueryLog[]>([]);
  const [isPaused, setIsPaused] = useState(false);
  const [search, setSearch] = useState("");
  const [selectedLog, setSelectedLog] = useState<XRayLog | null>(null);
  const [selectedDbLog, setSelectedDbLog] = useState<DBQueryLog | null>(null);

  const ws = useRef<WebSocket | null>(null);
  const logContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const socket = new WebSocket(`${protocol}//${host}/api/ws`);

    socket.onmessage = (event) => {
      const message = JSON.parse(event.data);
      if (!isPaused) {
        if (message.type === "xray:log") {
          setLogs((prev) => {
            const newLog = {
              ...message.data,
              id: Math.random().toString(36).substr(2, 9),
            };
            return [newLog, ...prev].slice(0, 100);
          });
        } else if (message.type === "database:query") {
          setDbLogs((prev) => {
            const newLog = message.data;
            return [newLog, ...prev].slice(0, 100);
          });
        }
      }
    };

    socket.onopen = () => console.log("X-Ray: Connected");
    socket.onclose = () => console.log("X-Ray: Disconnected");

    ws.current = socket;

    return () => {
      socket.close();
    };
  }, [isPaused]);

  const filteredLogs = logs.filter(
    (log) =>
      log.host.toLowerCase().includes(search.toLowerCase()) ||
      log.uri.toLowerCase().includes(search.toLowerCase()) ||
      log.method.toLowerCase().includes(search.toLowerCase())
  );

  const filteredDbLogs = dbLogs.filter(
    (log) =>
      log.argument.toLowerCase().includes(search.toLowerCase()) ||
      log.user_host.toLowerCase().includes(search.toLowerCase())
  );

  const clearLogs = () => {
    if (mode === "http") setLogs([]);
    else setDbLogs([]);
  };

  return (
    <div className="flex flex-col h-[calc(100vh-var(--topbar-height)-2rem)] space-y-4">
      {/* Header / Toolbar */}
      <div className="flex items-center justify-between bg-[var(--card)] p-4 rounded-xl border border-[var(--border)] shadow-sm">
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 rounded-lg bg-blue-500/10 flex items-center justify-center text-blue-500">
            <Zap size={24} fill="currentColor" />
          </div>
          <div>
            <h1 className="text-xl font-bold">X-Ray Profiler</h1>
            <p className="text-sm text-[var(--muted-foreground)]">
              {mode === "http" ? "Real-time HTTP traffic monitoring" : "Real-time Database Query Watcher"}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* Mode Switcher */}
          <div className="flex bg-[var(--background)] border border-[var(--border)] rounded-lg p-1 mr-2">
            <button
              onClick={() => setMode("http")}
              className={cn(
                "flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-colors",
                mode === "http" ? "bg-[var(--primary)] text-primary-foreground shadow-sm" : "text-[var(--muted-foreground)] hover:text-foreground"
              )}
            >
              <Globe size={14} /> HTTP
            </button>
            <button
              onClick={() => setMode("database")}
              className={cn(
                "flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-colors",
                mode === "database" ? "bg-[var(--primary)] text-primary-foreground shadow-sm" : "text-[var(--muted-foreground)] hover:text-foreground"
              )}
            >
              <Database size={14} /> Queries
            </button>
          </div>

          {/* Search */}
          <div className="relative group">
            <Search
              className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)] group-focus-within:text-blue-500 transition-colors"
              size={16}
            />
            <input
              type="text"
              placeholder="Search traffic..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 pr-4 py-2 bg-[var(--background)] border border-[var(--border)] rounded-lg text-sm w-64 focus:outline-none focus:ring-2 focus:ring-blue-500/20 transition-all"
            />
          </div>

          <div className="h-6 w-px bg-[var(--border)] mx-1" />

          {/* Controls */}
          <button
            onClick={() => setIsPaused(!isPaused)}
            className={cn(
              "flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors",
              isPaused
                ? "bg-amber-500/10 text-amber-500 hover:bg-amber-500/20"
                : "bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20"
            )}
          >
            {isPaused ? (
              <Play size={16} fill="currentColor" />
            ) : (
              <Pause size={16} fill="currentColor" />
            )}
            {isPaused ? "Resume" : "Live"}
          </button>

          <button
            onClick={clearLogs}
            className="p-2 text-[var(--muted-foreground)] hover:text-red-500 hover:bg-red-500/10 rounded-lg transition-colors"
            title="Clear logs"
          >
            <Trash2 size={20} />
          </button>
        </div>
      </div>

      <div className="flex-1 flex gap-4 overflow-hidden">
        {mode === "http" ? (
          <>
            {/* Main Log List */}
            <div className="flex-1 bg-[var(--card)] rounded-xl border border-[var(--border)] shadow-sm overflow-hidden flex flex-col">
              <div className="grid grid-cols-[100px_80px_1fr_80px_100px] gap-4 px-4 py-3 bg-[var(--background)] border-b border-[var(--border)] text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                <div>Time</div>
                <div>Method</div>
                <div>Request</div>
                <div className="text-center">Status</div>
                <div className="text-right">Latency</div>
              </div>

              <div className="flex-1 overflow-y-auto" ref={logContainerRef}>
                {filteredLogs.length === 0 ? (
                  <div className="h-full flex flex-col items-center justify-center text-[var(--muted-foreground)] opacity-50 space-y-4">
                    <Activity size={48} strokeWidth={1} />
                    <p>Waiting for traffic...</p>
                  </div>
                ) : (
                  filteredLogs.map((log) => (
                    <div
                      key={log.id}
                      onClick={() => setSelectedLog(log)}
                      className={cn(
                        "grid grid-cols-[100px_80px_1fr_80px_100px] gap-4 px-4 py-3 border-b border-[var(--border)] items-center cursor-pointer transition-colors hover:bg-[var(--card-hover)] animate-in fade-in slide-in-from-top-2 duration-300",
                        selectedLog?.id === log.id &&
                          "bg-[var(--card-hover)] border-l-2 border-l-blue-500"
                      )}
                    >
                      <div className="text-[11px] text-[var(--muted-foreground)] font-mono">
                        {new Date(log.time_iso).toLocaleTimeString([], {
                          hour12: false,
                          hour: "2-digit",
                          minute: "2-digit",
                          second: "2-digit",
                        })}
                      </div>
                      <div>
                        <span
                          className={cn(
                            "px-2 py-0.5 rounded-md text-[10px] font-bold uppercase",
                            log.method === "GET"
                              ? "bg-blue-500/10 text-blue-500"
                              : log.method === "POST"
                              ? "bg-emerald-500/10 text-emerald-500"
                              : "bg-amber-500/10 text-amber-500"
                          )}
                        >
                          {log.method}
                        </span>
                      </div>
                      <div className="flex flex-col min-w-0">
                        <span className="text-sm font-medium truncate">
                          {log.host}
                        </span>
                        <span className="text-xs text-[var(--muted-foreground)] truncate font-mono">
                          {log.uri}
                        </span>
                      </div>
                      <div className="flex justify-center">
                        <span
                          className={cn(
                            "px-2 py-0.5 rounded-full text-[11px] font-bold",
                            log.status < 300
                              ? "bg-emerald-500/10 text-emerald-500"
                              : log.status < 400
                              ? "bg-blue-500/10 text-blue-500"
                              : "bg-red-500/10 text-red-500"
                          )}
                        >
                          {log.status}
                        </span>
                      </div>
                      <div className="text-right text-[11px] font-mono text-[var(--muted-foreground)]">
                        {Number(log.latency).toFixed(3)}s
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>

            {/* Detail Panel */}
            {selectedLog && (
              <div className="w-96 bg-[var(--card)] rounded-xl border border-[var(--border)] shadow-lg animate-in slide-in-from-right-4 duration-300 flex flex-col">
                <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
                  <h2 className="font-bold flex items-center gap-2">
                    <ArrowUpRight size={18} className="text-blue-500" />
                    Request Details
                  </h2>
                  <button
                    onClick={() => setSelectedLog(null)}
                    className="p-1 hover:bg-[var(--card-hover)] rounded"
                  >
                    <ChevronRight size={20} />
                  </button>
                </div>

                <div className="flex-1 overflow-y-auto p-4 space-y-6">
                  <div className="space-y-3">
                    <div className="flex flex-col gap-1">
                      <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                        Full URL
                      </span>
                      <div className="p-2 bg-[var(--background)] rounded-lg text-xs break-all font-mono border border-[var(--border)]">
                        http://{selectedLog.host}
                        {selectedLog.uri}
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-4">
                      <div className="flex flex-col gap-1">
                        <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                          Status
                        </span>
                        <div className="text-sm font-bold flex items-center gap-2">
                          <span
                            className={cn(
                              "w-2 h-2 rounded-full",
                              selectedLog.status < 300
                                ? "bg-emerald-500"
                                : "bg-red-500"
                            )}
                          />
                          {selectedLog.status} OK
                        </div>
                      </div>
                      <div className="flex flex-col gap-1 text-right">
                        <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                          Latency
                        </span>
                        <div className="text-sm font-bold flex items-center gap-1 justify-end">
                          <Clock
                            size={14}
                            className="text-[var(--muted-foreground)]"
                          />
                          {Number(selectedLog.latency).toFixed(3)}s
                        </div>
                      </div>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                      Metadata
                    </span>
                    <div className="space-y-2">
                      <div className="flex justify-between text-xs py-1 border-b border-[var(--border)]">
                        <span className="text-[var(--muted-foreground)]">
                          IP Address
                        </span>
                        <span className="font-mono">127.0.0.1</span>
                      </div>
                      <div className="flex justify-between text-xs py-1 border-b border-[var(--border)]">
                        <span className="text-[var(--muted-foreground)]">
                          Payload Size
                        </span>
                        <span className="font-mono">
                          {selectedLog.body_bytes} bytes
                        </span>
                      </div>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                      User Agent
                    </span>
                    <div className="p-3 bg-[var(--background)] rounded-lg text-xs leading-relaxed text-[var(--muted-foreground)] border border-[var(--border)]">
                      {selectedLog.agent}
                    </div>
                  </div>
                </div>

                <div className="p-4 bg-[var(--background)] border-t border-[var(--border)] rounded-b-xl">
                  <p className="text-[10px] text-center text-[var(--muted-foreground)]">
                    Showing raw Nginx access log data as JSON
                  </p>
                </div>
              </div>
            )}
          </>
        ) : (
          <>
            {/* Database Query List */}
            <div className="flex-1 bg-[var(--card)] rounded-xl border border-[var(--border)] shadow-sm overflow-hidden flex flex-col">
              <div className="grid grid-cols-[100px_80px_120px_1fr] gap-4 px-4 py-3 bg-[var(--background)] border-b border-[var(--border)] text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                <div>Time</div>
                <div>Thread</div>
                <div>User/Host</div>
                <div>Query</div>
              </div>

              <div className="flex-1 overflow-y-auto" ref={logContainerRef}>
                {filteredDbLogs.length === 0 ? (
                  <div className="h-full flex flex-col items-center justify-center text-[var(--muted-foreground)] opacity-50 space-y-4">
                    <Database size={48} strokeWidth={1} />
                    <p>Waiting for database queries...</p>
                  </div>
                ) : (
                  filteredDbLogs.map((log) => (
                    <div
                      key={log.id}
                      onClick={() => setSelectedDbLog(log)}
                      className={cn(
                        "grid grid-cols-[100px_80px_120px_1fr] gap-4 px-4 py-3 border-b border-[var(--border)] items-center cursor-pointer transition-colors hover:bg-[var(--card-hover)] animate-in fade-in slide-in-from-top-2 duration-300",
                        selectedDbLog?.id === log.id &&
                          "bg-[var(--card-hover)] border-l-2 border-l-blue-500"
                      )}
                    >
                      <div className="text-[11px] text-[var(--muted-foreground)] font-mono">
                        {new Date(log.event_time).toLocaleTimeString([], {
                          hour12: false,
                          hour: "2-digit",
                          minute: "2-digit",
                          second: "2-digit",
                        })}
                      </div>
                      <div className="text-xs text-[var(--muted-foreground)]">
                        {log.thread_id}
                      </div>
                      <div className="text-xs text-[var(--muted-foreground)] truncate">
                        {log.user_host}
                      </div>
                      <div className="text-xs font-mono truncate text-[var(--primary)]">
                        {log.argument}
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>

            {/* DB Detail Panel */}
            {selectedDbLog && (
              <div className="w-[400px] bg-[var(--card)] rounded-xl border border-[var(--border)] shadow-lg animate-in slide-in-from-right-4 duration-300 flex flex-col">
                <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
                  <h2 className="font-bold flex items-center gap-2">
                    <Database size={18} className="text-blue-500" />
                    Query Details
                  </h2>
                  <button
                    onClick={() => setSelectedDbLog(null)}
                    className="p-1 hover:bg-[var(--card-hover)] rounded"
                  >
                    <ChevronRight size={20} />
                  </button>
                </div>

                <div className="flex-1 overflow-y-auto p-4 space-y-6">
                  <div className="space-y-3">
                    <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                      Raw SQL Query
                    </span>
                    <div className="p-3 bg-[var(--background)] rounded-lg text-sm font-mono border border-[var(--border)] overflow-x-auto whitespace-pre-wrap text-[var(--foreground)]">
                      {selectedDbLog.argument}
                    </div>
                  </div>

                  <div className="space-y-2">
                    <span className="text-[10px] uppercase tracking-wider font-bold text-[var(--muted-foreground)]">
                      Metadata
                    </span>
                    <div className="space-y-2">
                      <div className="flex justify-between text-xs py-1 border-b border-[var(--border)]">
                        <span className="text-[var(--muted-foreground)]">
                          Thread ID
                        </span>
                        <span className="font-mono">{selectedDbLog.thread_id}</span>
                      </div>
                      <div className="flex justify-between text-xs py-1 border-b border-[var(--border)]">
                        <span className="text-[var(--muted-foreground)]">
                          Server ID
                        </span>
                        <span className="font-mono">{selectedDbLog.server_id}</span>
                      </div>
                      <div className="flex justify-between text-xs py-1 border-b border-[var(--border)]">
                        <span className="text-[var(--muted-foreground)]">
                          Command
                        </span>
                        <span className="font-mono">{selectedDbLog.command}</span>
                      </div>
                      <div className="flex justify-between text-xs py-1 border-b border-[var(--border)]">
                        <span className="text-[var(--muted-foreground)]">
                          User / Host
                        </span>
                        <span className="font-mono text-right">{selectedDbLog.user_host}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
