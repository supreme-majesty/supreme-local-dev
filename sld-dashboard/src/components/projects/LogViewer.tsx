import { useEffect, useRef, useState } from "react";
import { Pause, Play, RefreshCw, X } from "lucide-react";
import { v2, type ProjectView } from "@/api/v2";
import { Button } from "@/components/ui/Button";

interface Props {
  project: ProjectView;
  onClose: () => void;
}

const MAX_CHARS = 400_000; // keep the browser responsive on chatty containers

/** Shows (and follows) the logs of one container of a project. */
export function LogViewer({ project, onClose }: Props) {
  const targets = (project.services ?? []).map((s) => s.app && s.service !== "nginx" ? s.app : s.service);
  const unique = Array.from(new Set(targets.length ? targets : (project.apps ?? []).map((a) => a.name)));
  const [target, setTarget] = useState(unique[0] ?? "");
  const [follow, setFollow] = useState(true);
  const [text, setText] = useState("");
  const [container, setContainer] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const bottom = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const abort = new AbortController();
    setText("");
    setError("");
    v2.logs(
      project.id,
      { target, tail: 300, follow, signal: abort.signal },
      (chunk) => setText((prev) => (prev + chunk).slice(-MAX_CHARS)),
    )
      .then(setContainer)
      .catch((e: Error) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => abort.abort();
  }, [project.id, target, follow, reloadKey]);

  useEffect(() => {
    if (follow) bottom.current?.scrollIntoView({ block: "end" });
  }, [text, follow]);

  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 justify-between px-4 py-2 border-b border-[var(--border)]">
        <div className="flex items-center gap-2 text-sm">
          <span className="font-medium text-[var(--foreground)]">Logs · {project.name}</span>
          <select
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            className="h-8 rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-xs text-[var(--foreground)]"
            aria-label="Container"
          >
            {unique.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
          {container && <span className="text-xs text-[var(--muted-foreground)]">{container}</span>}
        </div>
        <div className="flex items-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => setFollow((f) => !f)} title={follow ? "Pause" : "Follow"}>
            {follow ? <Pause size={14} /> : <Play size={14} />}
            {follow ? "Pause" : "Follow"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setReloadKey((k) => k + 1)} title="Reload">
            <RefreshCw size={14} />
          </Button>
          <Button size="sm" variant="ghost" onClick={onClose} aria-label="Close logs">
            <X size={14} />
          </Button>
        </div>
      </div>
      <pre className="h-96 overflow-auto p-4 text-xs leading-relaxed font-mono whitespace-pre-wrap break-words terminal bg-slate-950 text-slate-200">
        {error ? <span className="text-red-400">{error}</span> : text || "No output yet."}
        <div ref={bottom} />
      </pre>
    </div>
  );
}
