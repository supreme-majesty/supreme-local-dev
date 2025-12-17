import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface CardProps {
  children: ReactNode;
  className?: string;
  hover?: boolean;
}

export function Card({ children, className, hover = true }: CardProps) {
  return (
    <div
      className={cn(
        "bg-[var(--card)] border border-[var(--border)] rounded-xl p-6",
        "shadow-[var(--shadow)]",
        hover &&
          "transition-all duration-200 hover:shadow-[var(--shadow-md)] hover:border-[var(--muted-foreground)]/20",
        className
      )}
    >
      {children}
    </div>
  );
}

interface CardHeaderProps {
  title: string;
  description?: string;
  icon?: ReactNode;
  action?: ReactNode;
}

export function CardHeader({
  title,
  description,
  icon,
  action,
}: CardHeaderProps) {
  return (
    <div className="flex items-start justify-between mb-4">
      <div className="flex items-center gap-3">
        {icon && (
          <div className="p-2 rounded-lg bg-[var(--primary)]/10 text-[var(--primary)]">
            {icon}
          </div>
        )}
        <div>
          <h3 className="font-semibold text-[var(--foreground)]">{title}</h3>
          {description && (
            <p className="text-sm text-[var(--muted-foreground)]">
              {description}
            </p>
          )}
        </div>
      </div>
      {action && <div>{action}</div>}
    </div>
  );
}
