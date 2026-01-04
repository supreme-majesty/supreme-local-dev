import { useState, useEffect } from "react";
import { api } from "../../api/daemon";
import type { HealerIssue } from "../../api/daemon";

export function HealerNotification() {
  const [issues, setIssues] = useState<HealerIssue[]>([]);
  const [resolving, setResolving] = useState<string | null>(null);

  // Poll for issues
  useEffect(() => {
    const fetchIssues = async () => {
      try {
        const data = await api.getHealerIssues();
        setIssues(data);
      } catch (err) {
        // Silently ignore - healer might not be active
      }
    };

    fetchIssues();
    const interval = setInterval(fetchIssues, 5000); // Poll every 5s
    return () => clearInterval(interval);
  }, []);

  const handleResolve = async (id: string) => {
    setResolving(id);
    try {
      await api.resolveHealerIssue(id);
      setIssues((prev) => prev.filter((i) => i.id !== id));
    } catch (err) {
      console.error("Failed to resolve issue:", err);
    } finally {
      setResolving(null);
    }
  };

  const handleDismiss = (id: string) => {
    setIssues((prev) => prev.filter((i) => i.id !== id));
  };

  if (issues.length === 0) return null;

  const severityStyles = {
    critical:
      "border-l-red-500 bg-gradient-to-r from-red-500/10 to-zinc-900/95",
    warning:
      "border-l-amber-500 bg-gradient-to-r from-amber-500/10 to-zinc-900/95",
    info: "border-l-blue-500 bg-gradient-to-r from-blue-500/10 to-zinc-900/95",
  };

  const severityIcons = {
    critical: "🚨",
    warning: "⚠️",
    info: "ℹ️",
  };

  return (
    <div className="fixed bottom-5 right-5 z-[9999] flex flex-col gap-2.5 max-w-[400px]">
      {issues.map((issue) => (
        <div
          key={issue.id}
          className={`flex items-start gap-3 px-4 py-3.5 rounded-xl backdrop-blur-lg border border-white/10 shadow-2xl animate-in slide-in-from-right duration-300 border-l-[3px] ${
            severityStyles[issue.severity]
          }`}
        >
          <div className="text-2xl flex-shrink-0">
            {severityIcons[issue.severity]}
          </div>
          <div className="flex-1 min-w-0">
            <div className="font-semibold text-[0.95rem] text-white mb-1">
              {issue.title}
            </div>
            <div className="text-[0.85rem] text-white/70 leading-relaxed">
              {issue.description}
            </div>
          </div>
          <div className="flex items-center gap-2 flex-shrink-0">
            {issue.can_auto_fix && (
              <button
                className="px-3.5 py-1.5 bg-gradient-to-r from-green-500 to-green-600 rounded-md text-white text-sm font-medium hover:shadow-lg hover:shadow-green-500/30 hover:-translate-y-0.5 transition-all disabled:opacity-70 disabled:cursor-not-allowed"
                onClick={() => handleResolve(issue.id)}
                disabled={resolving === issue.id}
              >
                {resolving === issue.id ? "Fixing..." : "Fix It"}
              </button>
            )}
            <button
              className="px-2 py-1 bg-transparent border-none text-white/50 text-base cursor-pointer hover:text-white/90 transition-colors"
              onClick={() => handleDismiss(issue.id)}
              title="Dismiss"
            >
              ✕
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
