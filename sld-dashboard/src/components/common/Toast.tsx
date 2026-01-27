import { X, CheckCircle, XCircle, AlertCircle, Info } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/stores/useAppStore";

const icons = {
  success: CheckCircle,
  error: XCircle,
  warning: AlertCircle,
  info: Info,
};

const styles = {
  success: "bg-green-500/10 border-green-500/20 text-green-400",
  error: "bg-red-500/10 border-red-500/20 text-red-400",
  warning: "bg-amber-500/10 border-amber-500/20 text-amber-400",
  info: "bg-blue-500/10 border-blue-500/20 text-blue-400",
};

export function Toast() {
  const { toasts, removeToast } = useAppStore();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 max-w-sm">
      {toasts.map((toast) => {
        const Icon = icons[toast.type];

        return (
          <div
            key={toast.id}
            className={cn(
              "flex items-start gap-3 p-4 rounded-lg border shadow-lg",
              "animate-slide-in-right",
              styles[toast.type]
            )}
          >
            <Icon size={20} className="flex-shrink-0 mt-0.5" />

            <div className="flex-1 min-w-0">
              <p className="font-medium text-sm">{toast.title}</p>
              {toast.description && (
                <p className="text-sm opacity-80 mt-0.5 truncate">
                  {toast.description}
                </p>
              )}
            </div>

            <button
              onClick={() => removeToast(toast.id)}
              className="flex-shrink-0 p-1 rounded hover:bg-white/10 transition-colors"
            >
              <X size={16} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
