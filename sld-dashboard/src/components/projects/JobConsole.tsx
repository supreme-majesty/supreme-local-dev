import { useEffect, useRef, useState } from "react";
import { CheckCircle2, Loader2, X, XCircle } from "lucide-react";
import { v2, type JobView } from "@/api/v2";
import { cn } from "@/lib/utils";

interface Props {
  jobId: string;
  title: string;
  onClose: () => void;
  onFinished?: (job: JobView) => void;
}

/** Streams a background job's output live, like a terminal. */
export function JobConsole({ jobId, title, onClose, onFinished }: Props) {
  const [output, setOutput] = useState("");
  const [job, setJob] = useState<JobView | null>(null);
  const bottom = useRef<HTMLDivElement>(null);
  const finished = useRef(onFinished);
  finished.current = onFinished;

  useEffect(() => {
    setOutput("");
    setJob(null);
    return v2.followJob(
      jobId,
      (text) => setOutput((prev) => prev + text),
      (done) => {
        setJob(done);
        finished.current?.(done);
      },
    );
  }, [jobId]);

  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "end" });
  }, [output]);

  const status = job?.status ?? "running";

  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] overflow-hidden">
      <div className="flex items-center justify-between px-4 py-2 border-b border-[var(--border)]">
        <div className="flex items-center gap-2 text-sm font-medium text-[var(--foreground)]">
          {status === "running" && <Loader2 size={16} className="animate-spin text-blue-400" />}
          {status === "succeeded" && <CheckCircle2 size={16} className="text-green-400" />}
          {status === "failed" && <XCircle size={16} className="text-red-400" />}
          {title}
          <span className="text-xs font-normal text-[var(--muted-foreground)]">
            {status === "running" ? "running…" : status}
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-1 rounded text-[var(--muted-foreground)] hover:text-[var(--foreground)] hover:bg-[var(--card-hover)]"
          aria-label="Close output"
        >
          <X size={16} />
        </button>
      </div>
      <pre
        className={cn(
          "max-h-80 overflow-auto p-4 text-xs leading-relaxed font-mono whitespace-pre-wrap break-words",
          "terminal bg-slate-950 text-slate-200",
        )}
      >
        {output || (status === "running" ? "Waiting for output…" : "")}
        {job?.error && <span className="text-red-400">{`\n✗ ${job.error}`}</span>}
        <div ref={bottom} />
      </pre>
    </div>
  );
}
