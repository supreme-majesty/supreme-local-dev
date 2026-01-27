import { Modal } from "@/components/ui/Modal";
import { usePluginLogs } from "@/hooks/use-daemon";
import { RefreshCw, ScrollText, AlertCircle } from "lucide-react";
import { useEffect, useRef } from "react";

interface PluginLogsModalProps {
  isOpen: boolean;
  onClose: () => void;
  pluginId: string;
  pluginName: string;
}

export function PluginLogsModal({
  isOpen,
  onClose,
  pluginId,
  pluginName,
}: PluginLogsModalProps) {
  const {
    data: logs = [],
    isLoading,
    isError,
    refetch,
  } = usePluginLogs(pluginId, 100, isOpen);

  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen && logs.length > 0) {
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, isOpen]);

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={`${pluginName} Logs`}>
      <div className="flex flex-col h-[500px] -mx-6 -my-4">
        <div className="flex items-center justify-between px-4 py-2 bg-[var(--muted)]/20 border-b border-[var(--border)]">
          <div className="flex items-center gap-2 text-sm text-[var(--muted-foreground)]">
            <ScrollText size={14} />
            <span>Last 100 lines</span>
          </div>
          <button
            onClick={() => refetch()}
            className="p-1.5 hover:bg-[var(--muted)] rounded-md text-[var(--muted-foreground)] hover:text-[var(--foreground)] transition-colors"
            title="Refresh Logs"
          >
            <RefreshCw size={14} className={isLoading ? "animate-spin" : ""} />
          </button>
        </div>

        <div className="flex-1 overflow-auto p-4 bg-[#1e1e1e] text-gray-300 font-mono text-xs">
          {isLoading && logs.length === 0 ? (
            <div className="flex justify-center items-center h-full">
              <RefreshCw className="animate-spin text-gray-500" />
            </div>
          ) : isError ? (
            <div className="flex justify-center items-center h-full text-red-400 gap-2">
              <AlertCircle size={16} />
              <span>Failed to load logs</span>
            </div>
          ) : logs.length === 0 ? (
            <div className="flex justify-center items-center h-full text-gray-600">
              No logs available
            </div>
          ) : (
            logs.map((line, i) => (
              <div
                key={i}
                className="whitespace-pre-wrap hover:bg-[#2a2a2a] px-1"
              >
                {line}
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      </div>
    </Modal>
  );
}
