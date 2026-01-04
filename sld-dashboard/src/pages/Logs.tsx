import { useEffect, useState, useRef } from "react";
import {
  Search,
  Trash2,
  Play,
  Pause,
  ChevronDown,
  Activity,
  Terminal,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

interface LogSource {
  id: string;
  label: string;
  path: string;
}

interface LogEntry {
  id: string;
  source: string;
  level: "debug" | "info" | "warning" | "error" | "unknown";
  message: string;
  timestamp: string;
  raw: string;
}

const COLORS = {
  error: "text-red-500 bg-red-500/10 border-red-500/20",
  warning: "text-amber-500 bg-amber-500/10 border-amber-500/20",
  info: "text-blue-500 bg-blue-500/10 border-blue-500/20",
  debug: "text-purple-500 bg-purple-500/10 border-purple-500/20",
  unknown: "text-gray-500 bg-gray-500/10 border-gray-500/20",
};

export default function Logs() {
  const [sources, setSources] = useState<LogSource[]>([]);
  const [activeSource, setActiveSource] = useState<string | null>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [isPaused, setIsPaused] = useState(false);
  const [search, setSearch] = useState("");

  const ws = useRef<WebSocket | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const shouldAutoScroll = useRef(true);

  // Fetch available log sources
  useEffect(() => {
    fetch("/api/logs/sources")
      .then((res) => res.json())
      .then((data) => {
        setSources(data);
        if (data.length > 0) {
          setActiveSource(data[0].id);
        }
      });
  }, []);

  // WebSocket connection
  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const socket = new WebSocket(`${protocol}//${host}/api/ws`);

    socket.onmessage = (event) => {
      const message = JSON.parse(event.data);
      if (message.type === "log:entry" && !isPaused) {
        // Only show logs for active source
        if (activeSource && message.data.source !== activeSource) return;

        setLogs((prev) => {
          const updated = [...prev, message.data].slice(-1000); // Keep last 1000
          return updated;
        });
      }
    };

    ws.current = socket;
    return () => {
      socket.close();
    };
  }, [activeSource, isPaused]);

  // Handle source switching
  useEffect(() => {
    if (!activeSource) return;

    // Start watching active source
    fetch("/api/logs/watch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ source: activeSource }),
    });

    setLogs([]); // Clear logs on switch

    return () => {
      // Unwatch previous source
      fetch("/api/logs/unwatch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source: activeSource }),
      });
    };
  }, [activeSource]);

  // Auto-scroll
  useEffect(() => {
    if (shouldAutoScroll.current && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs]);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const { scrollTop, scrollHeight, clientHeight } = e.currentTarget;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
    shouldAutoScroll.current = isAtBottom;
  };

  const filteredLogs = logs.filter((log) =>
    log.message.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="flex flex-col h-[calc(100vh-var(--topbar-height)-2rem)] space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between bg-[var(--card)] p-4 rounded-xl border border-[var(--border)] shadow-sm">
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center text-purple-500">
            <Terminal size={24} />
          </div>
          <div>
            <h1 className="text-xl font-bold">System Logs</h1>
            <p className="text-sm text-[var(--muted-foreground)]">
              Real-time server log viewer
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* Source Selector */}
          <div className="relative">
            <select
              value={activeSource || ""}
              onChange={(e) => setActiveSource(e.target.value)}
              className="appearance-none pl-4 pr-10 py-2 bg-[var(--background)] border border-[var(--border)] rounded-lg text-sm font-medium focus:outline-none focus:ring-2 focus:ring-purple-500/20 cursor-pointer min-w-[180px]"
            >
              {sources.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.label}
                </option>
              ))}
            </select>
            <ChevronDown
              size={14}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)] pointer-events-none"
            />
          </div>

          <div className="h-6 w-px bg-[var(--border)] mx-1" />

          {/* Search */}
          <div className="relative group">
            <Search
              className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)] group-focus-within:text-purple-500 transition-colors"
              size={16}
            />
            <input
              type="text"
              placeholder="Filter logs..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 pr-4 py-2 bg-[var(--background)] border border-[var(--border)] rounded-lg text-sm w-64 focus:outline-none focus:ring-2 focus:ring-purple-500/20 transition-all"
            />
          </div>

          {/* Controls */}
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setIsPaused(!isPaused)}
            className={cn(
              "gap-2",
              isPaused && "text-amber-500 bg-amber-500/10 hover:bg-amber-500/20"
            )}
          >
            {isPaused ? <Play size={16} /> : <Pause size={16} />}
            {isPaused ? "Resume" : "Pause"}
          </Button>

          <Button
            variant="ghost"
            size="sm"
            onClick={() => setLogs([])}
            title="Clear logs"
            className="w-9 px-0"
          >
            <Trash2 size={18} />
          </Button>
        </div>
      </div>

      {/* Log Console */}
      <div className="flex-1 bg-[var(--card)] rounded-xl border border-[var(--border)] shadow-sm overflow-hidden flex flex-col font-mono text-sm relative">
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="flex-1 overflow-y-auto p-4 space-y-1"
        >
          {filteredLogs.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-[var(--muted-foreground)] opacity-50 space-y-4">
              <Activity size={48} strokeWidth={1} />
              <p>Waiting for logs from {activeSource}...</p>
            </div>
          ) : (
            filteredLogs.map((log) => (
              <div
                key={log.id}
                className="flex gap-4 hover:bg-[var(--card-hover)] px-2 py-1 -mx-2 rounded"
              >
                <div className="text-[var(--muted-foreground)] shrink-0 w-[140px] text-xs pt-0.5">
                  {new Date(log.timestamp).toLocaleTimeString([], {
                    hour12: false,
                    hour: "2-digit",
                    minute: "2-digit",
                    second: "2-digit",
                    fractionalSecondDigits: 3,
                  })}
                </div>
                <div
                  className={cn(
                    "uppercase text-[10px] font-bold px-1.5 py-0.5 rounded h-fit shrink-0 w-16 text-center select-none",
                    COLORS[log.level] || COLORS.unknown
                  )}
                >
                  {log.level}
                </div>
                <div className="break-all whitespace-pre-wrap flex-1 text-[var(--foreground)] opacity-90">
                  {log.message}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Pause Indicator */}
        {isPaused && (
          <div className="absolute top-4 right-8 bg-amber-500 text-white text-xs font-bold px-3 py-1 rounded-full shadow-lg animate-pulse flex items-center gap-2 pointer-events-none">
            <Pause size={12} fill="currentColor" />
            PAUSED
          </div>
        )}
      </div>
    </div>
  );
}
