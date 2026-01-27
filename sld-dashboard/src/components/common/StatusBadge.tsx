import { cn } from "@/lib/utils";

interface StatusBadgeProps {
  status: "running" | "stopped" | "loading";
  className?: string;
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const config = {
    running: {
      label: "Running",
      dotClass: "bg-green-500 status-dot running",
      bgClass: "bg-green-500/10",
      textClass: "text-green-400",
    },
    stopped: {
      label: "Stopped",
      dotClass: "bg-red-500 status-dot stopped",
      bgClass: "bg-red-500/10",
      textClass: "text-red-400",
    },
    loading: {
      label: "Loading...",
      dotClass: "bg-amber-500 animate-pulse",
      bgClass: "bg-amber-500/10",
      textClass: "text-amber-400",
    },
  }[status];

  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 py-1.5 rounded-full",
        config.bgClass,
        className
      )}
    >
      <span className={cn("w-2.5 h-2.5 rounded-full", config.dotClass)} />
      <span className={cn("text-sm font-medium", config.textClass)}>
        {config.label}
      </span>
    </div>
  );
}
