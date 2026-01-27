import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface SystemInfoCardProps {
  icon: ReactNode;
  label: string;
  value: string;
  subValue?: string;
}

export function SystemInfoCard({
  icon,
  label,
  value,
  subValue,
}: SystemInfoCardProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-4 p-4 rounded-lg",
        "bg-[var(--muted)]/30 border border-[var(--border)]"
      )}
    >
      <div className="p-2.5 rounded-lg bg-[var(--secondary)] text-[var(--muted-foreground)]">
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs text-[var(--muted-foreground)] uppercase tracking-wider">
          {label}
        </p>
        <p className="font-semibold text-[var(--foreground)] font-mono truncate">
          {value}
        </p>
        {subValue && (
          <p className="text-xs text-[var(--muted-foreground)] truncate">
            {subValue}
          </p>
        )}
      </div>
    </div>
  );
}
