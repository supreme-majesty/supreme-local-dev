import { useState, useEffect, useRef, type ReactNode } from "react";
import { cn } from "@/lib/utils";

interface ContextMenuItem {
  label: string;
  icon?: ReactNode;
  onClick: () => void;
  variant?: "default" | "danger";
}

interface ContextMenuProps {
  x: number;
  y: number;
  onClose: () => void;
  items: ContextMenuItem[];
}

export function ContextMenu({ x, y, onClose, items }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    document.addEventListener("wheel", onClose);
    document.addEventListener("resize", onClose);

    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("wheel", onClose);
      document.removeEventListener("resize", onClose);
    };
  }, [onClose]);

  // Adjust position if menu goes off screen
  const [position, setPosition] = useState({ top: y, left: x });

  useEffect(() => {
    if (menuRef.current) {
      const rect = menuRef.current.getBoundingClientRect();
      const winWidth = window.innerWidth;
      const winHeight = window.innerHeight;

      let newLeft = x;
      let newTop = y;

      if (x + rect.width > winWidth) {
        newLeft = x - rect.width;
      }

      if (y + rect.height > winHeight) {
        newTop = y - rect.height;
      }

      setPosition({ top: newTop, left: newLeft });
    }
  }, [x, y]);

  return (
    <div
      ref={menuRef}
      className="fixed z-50 min-w-[160px] bg-[var(--card)] border border-[var(--border)] rounded-md shadow-lg py-1 animate-in fade-in zoom-in-95 duration-100"
      style={{ top: position.top, left: position.left }}
    >
      {items.map((item, index) => (
        <button
          key={index}
          className={cn(
            "flex items-center gap-2 w-full px-3 py-1.5 text-sm text-left transition-colors",
            item.variant === "danger"
              ? "text-red-500 hover:bg-red-500/10"
              : "text-[var(--foreground)] hover:bg-[var(--muted)]",
          )}
          onClick={(e) => {
            e.stopPropagation();
            item.onClick();
            onClose();
          }}
        >
          {item.icon && (
            <span className="text-[var(--muted-foreground)] group-hover:text-inherit">
              {item.icon}
            </span>
          )}
          <span>{item.label}</span>
        </button>
      ))}
    </div>
  );
}
