import { useEffect, useMemo, useState, useRef } from "react";
import {
  Search,
  Trash2,
  Play,
  Pause,
  ChevronDown,
  Activity,
  Terminal,
  AlertTriangle,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

/** A container whose logs can be read (SLD's services, share tunnels, project containers). */
interface LogSource {
  container: string;
  label: string;
  group: string;
  state: string;
}

interface LogEntry {
  id: string;
  level: "debug" | "info" | "warning" | "error" | "unknown";
  message: string;
  timestamp: string;
}

const COLORS = {
  error: "text-red-500 bg-red-500/10 border-red-500/20",
  warning: "text-amber-500 bg-amber-500/10 border-amber-500/20",
  info: "text-blue-500 bg-blue-500/10 border-blue-500/20",
  debug: "text-purple-500 bg-purple-500/10 border-purple-500/20",
  unknown: "text-gray-500 bg-gray-500/10 border-gray-500/20",
};

const MAX_LINES = 1000;

// levelOf guesses a line's level from the usual markers (Laravel, nginx, PHP, Traefik, MySQL, Node...).
function levelOf(line: string): LogEntry["level"] {
  const l = line.toLowerCase();
  if (/\b(error|err|fatal|crit(ical)?|emerg(ency)?|alert|exception|panic)\b|"level":"error"|\.error:/.test(l)) return "error";
  if (/\b(warn(ing)?)\b|"level":"warn|\.warning:/.test(l)) return "warning";
  if (/\b(debug|trace)\b|"level":"debug"/.test(l)) return "debug";
  if (/\b(info|notice)\b|"level":"info"|\.info:/.test(l)) return "info";
  return "unknown";
}

let lineCounter = 0;

// parseLine splits Docker's "<RFC3339 timestamp> <message>" format.
function parseLine(raw: string): LogEntry {
  const space = raw.indexOf(" ");
  const ts = space > 0 ? raw.slice(0, space) : "";
  const valid = ts && !Number.isNaN(Date.parse(ts));
  const message = valid ? raw.slice(space + 1) : raw;
  return {
    id: `l${++lineCounter}`,
    level: levelOf(message),
    message,
    timestamp: valid ? ts : new Date().toISOString(),
  };
}

export default function Logs() {
  const [activeSource, setActiveSource] = useState<string | null>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [isPaused, setIsPaused] = useState(false);
  const [search, setSearch] = useState("");
  const [streamError, setStreamError] = useState<string | null>(null);

  const paused = useRef(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const shouldAutoScroll = useRef(true);

  const sourcesQuery = useQuery({
    queryKey: ["v2", "log-sources"],
    queryFn: async () => {
      const res = await fetch("/api/v2/logs/sources");
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || res.statusText);
      return body as LogSource[];
    },
    refetchInterval: 15000,
  });
  const sources = useMemo(() => sourcesQuery.data ?? [], [sourcesQuery.data]);
  const groups = useMemo(() => {
    const byGroup = new Map<string, LogSource[]>();
    for (const s of sources) byGroup.set(s.group, [...(byGroup.get(s.group) ?? []), s]);
    return [...byGroup.entries()];
  }, [sources]);
  const active = sources.find((s) => s.container === activeSource);

  // Default to the first source (the control plane or router).
  useEffect(() => {
    if (!activeSource && sources.length > 0) setActiveSource(sources[0].container);
  }, [sources, activeSource]);

  useEffect(() => {
    paused.current = isPaused;
  }, [isPaused]);

  // Stream the active container's logs: the last lines, then new ones as they come.
  useEffect(() => {
    if (!activeSource) return;
    const controller = new AbortController();
    setLogs([]);
    setStreamError(null);
    (async () => {
      try {
        const res = await fetch(
          `/api/v2/logs/stream?container=${encodeURIComponent(activeSource)}&tail=300&follow=1`,
          { signal: controller.signal },
        );
        if (!res.ok || !res.body) {
          const body = await res.json().catch(() => ({}));
          throw new Error(body.error || res.statusText);
        }
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let pending = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          pending += decoder.decode(value, { stream: true });
          const lines = pending.split("\n");
          pending = lines.pop() ?? "";
          if (paused.current) continue;
          const entries = lines.filter((l) => l.trim() !== "").map(parseLine);
          if (entries.length) setLogs((prev) => [...prev, ...entries].slice(-MAX_LINES));
        }
      } catch (e) {
        if (!controller.signal.aborted) setStreamError((e as Error).message);
      }
    })();
    return () => controller.abort();
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
              Live logs of SLD&apos;s services and your projects&apos; containers
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* Source Selector */}
          <div className="relative">
            <select
              value={activeSource || ""}
              onChange={(e) => setActiveSource(e.target.value)}
              className="appearance-none pl-4 pr-10 py-2 bg-[var(--background)] border border-[var(--border)] rounded-lg text-sm font-medium focus:outline-none focus:ring-2 focus:ring-purple-500/20 cursor-pointer min-w-[220px]"
              aria-label="Log source"
            >
              {sources.length === 0 && <option value="">No containers</option>}
              {groups.map(([group, items]) => (
                <optgroup key={group} label={group}>
                  {items.map((s) => (
                    <option key={s.container} value={s.container}>
                      {s.label}
                      {s.state !== "running" ? ` (${s.state})` : ""}
                    </option>
                  ))}
                </optgroup>
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
          {(streamError || sourcesQuery.error) && (
            <div className="mb-3 flex items-start gap-2 rounded-lg border border-red-500/30 bg-red-500/10 p-3 font-sans text-sm text-red-400">
              <AlertTriangle size={16} className="mt-0.5 flex-shrink-0" />
              {streamError || (sourcesQuery.error as Error).message}
            </div>
          )}
          {filteredLogs.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-[var(--muted-foreground)] opacity-50 space-y-4">
              <Activity size={48} strokeWidth={1} />
              <p>
                {sourcesQuery.isLoading
                  ? "Loading log sources..."
                  : active
                  ? `Waiting for logs from ${active.group} / ${active.label}...`
                  : "No containers to show logs for."}
              </p>
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
