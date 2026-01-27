import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface QuickActionCardProps {
  icon: ReactNode;
  title: string;
  description: string;
  onClick: () => void;
  variant?: "default" | "primary";
}

export function QuickActionCard({
  icon,
  title,
  description,
  onClick,
  variant = "default",
}: QuickActionCardProps) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "w-full text-left p-4 rounded-lg border",
        "transition-all duration-200 group",
        "hover:shadow-[var(--shadow-md)] active:scale-[0.98]",
        variant === "primary"
          ? "bg-gradient-to-br from-blue-500/10 to-indigo-500/10 border-blue-500/20 hover:border-blue-500/40"
          : "bg-[var(--muted)]/30 border-[var(--border)] hover:border-[var(--muted-foreground)]/30 hover:bg-[var(--muted)]/50"
      )}
    >
      <div className="flex items-center gap-3">
        <div
          className={cn(
            "p-2.5 rounded-lg transition-transform duration-200 group-hover:scale-110",
            variant === "primary"
              ? "bg-blue-500/20 text-blue-400"
              : "bg-[var(--secondary)] text-[var(--muted-foreground)]"
          )}
        >
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <p className="font-medium text-[var(--foreground)] truncate">
            {title}
          </p>
          <p className="text-xs text-[var(--muted-foreground)] truncate">
            {description}
          </p>
        </div>
      </div>
    </button>
  );
}
