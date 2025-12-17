import { cn } from "@/lib/utils";
import type { ServiceStatus } from "@/api/daemon";

interface ServiceCardProps {
  service: ServiceStatus;
}

export function ServiceCard({ service }: ServiceCardProps) {
  return (
    <div
      className={cn(
        "flex items-center justify-between p-4 rounded-lg",
        "bg-[var(--muted)]/30 border border-[var(--border)]",
        "transition-all duration-200 hover:bg-[var(--muted)]/50"
      )}
    >
      <div className="flex items-center gap-3">
        <div
          className={cn(
            "w-3 h-3 rounded-full status-dot",
            service.running ? "bg-green-500 running" : "bg-red-500 stopped"
          )}
        />
        <div>
          <p className="font-medium text-[var(--foreground)]">{service.name}</p>
          {service.version && (
            <p className="text-xs text-[var(--muted-foreground)] font-mono">
              v{service.version}
            </p>
          )}
        </div>
      </div>
      <span
        className={cn(
          "text-xs font-medium px-2 py-1 rounded",
          service.running
            ? "bg-green-500/10 text-green-400"
            : "bg-red-500/10 text-red-400"
        )}
      >
        {service.running ? "Active" : "Stopped"}
      </span>
    </div>
  );
}
