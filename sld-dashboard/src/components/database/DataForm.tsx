import { useState, useEffect } from "react";
import { type ColumnInfo } from "../../api/daemon";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { Checkbox } from "@/components/ui/Checkbox";

interface DataFormProps {
  columns: ColumnInfo[];
  initialData?: Record<string, unknown>;
  onSubmit: (
    data: Record<string, unknown>,
    mode?: "save" | "save_and_add"
  ) => void;
  isLoading?: boolean;
  onCancel?: () => void;
}

export function DataForm({
  columns,
  initialData,
  onSubmit,
  isLoading = false,
  onCancel,
}: DataFormProps) {
  const [formData, setFormData] = useState<Record<string, unknown>>(
    initialData || {}
  );

  useEffect(() => {
    if (initialData) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setFormData(initialData);
    }
  }, [initialData]);

  const handleSubmit = (e: React.FormEvent, mode: "save" | "save_and_add") => {
    e.preventDefault();
    onSubmit(formData, mode);
  };

  const handleChange = (column: string, value: unknown) => {
    setFormData((prev) => ({ ...prev, [column]: value }));
  };

  return (
    <form onSubmit={(e) => handleSubmit(e, "save")} className="space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {columns.map((col) => (
          <div key={col.name} className="space-y-2">
            <Label htmlFor={col.name} className="flex items-center gap-2">
              {col.name}
              {col.key === "PRI" && (
                <span className="text-[10px] bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200 px-1 rounded">
                  PK
                </span>
              )}
              {col.foreign_key && (
                <span className="text-[10px] bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 px-1 rounded">
                  FK
                </span>
              )}
            </Label>
            {col.type.includes("text") || col.type.includes("blob") ? (
              <textarea
                id={col.name}
                className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                value={String(formData[col.name] ?? "")}
                onChange={(e) => handleChange(col.name, e.target.value)}
                disabled={isLoading}
              />
            ) : col.type.includes("bool") || col.type.includes("tinyint(1)") ? (
              <div className="flex items-center space-x-2">
                <Checkbox
                  id={col.name}
                  checked={Boolean(formData[col.name])}
                  onChange={(e) => handleChange(col.name, e.target.checked)}
                  disabled={isLoading}
                />
                <label
                  htmlFor={col.name}
                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                >
                  True/False
                </label>
              </div>
            ) : (
              <Input
                id={col.name}
                type={
                  col.type.includes("int") ||
                  col.type.includes("decimal") ||
                  col.type.includes("float")
                    ? "number"
                    : "text"
                }
                value={String(formData[col.name] ?? "")}
                onChange={(e) => handleChange(col.name, e.target.value)}
                disabled={isLoading}
                placeholder={col.default ? `Default: ${col.default}` : ""}
              />
            )}
            <div className="text-xs text-[var(--muted-foreground)]">
              {col.type} {col.null === "YES" ? "(Nullable)" : "(Required)"}
            </div>
          </div>
        ))}
      </div>

      <div className="pt-4 flex items-center justify-end gap-2 border-t border-[var(--border)]">
        {onCancel && (
          <Button
            type="button"
            variant="ghost"
            onClick={onCancel}
            disabled={isLoading}
          >
            Cancel
          </Button>
        )}
        <Button
          type="button"
          variant="secondary"
          onClick={(e) => handleSubmit(e, "save_and_add")}
          disabled={isLoading}
        >
          Save & Add Another
        </Button>
        <Button type="submit" disabled={isLoading}>
          {isLoading ? "Saving..." : "Save"}
        </Button>
      </div>
    </form>
  );
}
