import { useState, useEffect, useRef } from "react";
import {
  Terminal,
  X,
  Play,
  Trash2,
  ChevronRight,
  Maximize2,
  Minimize2,
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { v2 } from "@/api/v2";
import { useArtisanCommands } from "@/hooks/use-daemon";

interface ConsoleDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  projectPath: string;
  projectName: string;
}

export function ConsoleDrawer({
  isOpen,
  onClose,
  projectPath,
  projectName,
}: ConsoleDrawerProps) {
  const [input, setInput] = useState("");
  const [output, setOutput] = useState<{ line: string; isError: boolean }[]>(
    []
  );
  const [isRunning, setIsRunning] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);

  const bottomRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const { data: commands = [] } = useArtisanCommands();
  const stopFollowing = useRef<(() => void) | null>(null);

  // Stop streaming when the drawer closes or unmounts.
  useEffect(() => {
    if (!isOpen) stopFollowing.current?.();
    return () => stopFollowing.current?.();
  }, [isOpen]);

  const finish = (success: boolean, error?: string) => {
    setIsRunning(false);
    setOutput((prev) => [
      ...prev,
      {
        line: success ? "Command finished successfully." : `Command failed${error ? `: ${error}` : "."}`,
        isError: !success,
      },
    ]);
    setTimeout(() => inputRef.current?.focus(), 100);
  };

  // Auto-scroll to bottom
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [output]);

  // Focus input on open
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 300);
    }
  }, [isOpen]);

  const handleRun = (cmd: string) => {
    if (!cmd.trim() || isRunning) return;

    setIsRunning(true);
    setOutput((prev) => [
      ...prev,
      { line: `> php artisan ${cmd}`, isError: false },
    ]);
    setInput("");

    // Runs in the project's PHP container; the output streams from the job.
    fetch("/api/artisan/run", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ project_path: projectPath, command: cmd }),
    })
      .then(async (res) => {
        const body = await res.json().catch(() => ({}));
        if (!res.ok || !body.job_id) throw new Error(body.error || res.statusText);
        stopFollowing.current?.();
        stopFollowing.current = v2.followJob(
          body.job_id,
          (text) =>
            setOutput((prev) => [
              ...prev,
              ...text.replace(/\n$/, "").split("\n").map((line) => ({ line, isError: false })),
            ]),
          (job) => finish(job.status === "succeeded", job.error),
        );
      })
      .catch((e: Error) => finish(false, e.message));
  };

  if (!isOpen) return null;

  return (
    <div
      className={`fixed bottom-0 left-0 right-0 bg-[#1e1e1e] text-white shadow-2xl border-t border-[#333] transition-all duration-300 z-50 flex flex-col ${
        isExpanded ? "h-[80vh]" : "h-[400px]"
      }`}
    >
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-[#252526] border-b border-[#333]">
        <div className="flex items-center gap-2">
          <Terminal size={16} className="text-[var(--primary)]" />
          <span className="font-mono text-sm font-medium">
            Artisan Console - {projectName}
          </span>
          {isRunning && (
            <span className="text-xs text-yellow-500 animate-pulse ml-2">
              ● Running...
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setOutput([])}
            className="p-1.5 hover:bg-[#333] rounded text-gray-400 hover:text-white transition-colors"
            title="Clear output"
          >
            <Trash2 size={14} />
          </button>
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="p-1.5 hover:bg-[#333] rounded text-gray-400 hover:text-white transition-colors"
            title={isExpanded ? "Minimize" : "Maximize"}
          >
            {isExpanded ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
          </button>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-[#333] rounded text-gray-400 hover:text-white transition-colors"
            title="Close"
          >
            <X size={14} />
          </button>
        </div>
      </div>

      {/* Main Area */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar - Quick Commands */}
        <div className="w-48 bg-[#252526] border-r border-[#333] p-2 overflow-y-auto hidden sm:block">
          <div className="text-xs font-bold text-gray-500 uppercase tracking-wider mb-2 px-2 mt-2">
            Quick Commands
          </div>
          <div className="space-y-0.5">
            {commands.map((cmd) => (
              <button
                key={cmd}
                onClick={() => handleRun(cmd)}
                disabled={isRunning}
                className="w-full text-left px-2 py-1.5 rounded text-xs text-gray-300 hover:bg-[#37373d] hover:text-white truncate transition-colors font-mono disabled:opacity-50"
              >
                {cmd}
              </button>
            ))}
          </div>
        </div>

        {/* Output Area */}
        <div className="flex-1 flex flex-col min-w-0 bg-[#1e1e1e]">
          <div className="flex-1 overflow-y-auto p-4 font-mono text-xs sm:text-sm space-y-1">
            <div className="text-gray-500 mb-2">
              Type a command or select one from the sidebar. (e.g. 'migrate',
              'route:list')
            </div>
            {output.map((line, i) => (
              <div
                key={i}
                className={`${
                  line.isError ? "text-red-400" : "text-gray-300"
                } whitespace-pre-wrap break-all leading-tight`}
              >
                {line.line}
              </div>
            ))}
            <div ref={bottomRef} />
          </div>

          {/* Input Area */}
          <div className="p-3 bg-[#252526] border-t border-[#333]">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleRun(input);
              }}
              className="flex items-center gap-2"
            >
              <ChevronRight
                size={16}
                className="text-[var(--primary)] shrink-0"
              />
              <input
                ref={inputRef}
                type="text"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                disabled={isRunning}
                placeholder="Enter command..."
                className="flex-1 bg-transparent border-none focus:outline-none text-white font-mono text-sm placeholder-gray-600"
                autoComplete="off"
                spellCheck={false}
              />
              <Button
                type="submit"
                size="sm"
                disabled={!input.trim() || isRunning}
                className="shrink-0 h-7"
              >
                <Play size={12} className="mr-1" />
                Run
              </Button>
            </form>
          </div>
        </div>
      </div>
    </div>
  );
}
