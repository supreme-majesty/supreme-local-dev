import { useState, useEffect } from "react";
import type { ColumnInfo } from "@/api/daemon";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Checkbox } from "@/components/ui/Checkbox";

interface DataFormProps {
  columns: ColumnInfo[];
  initialData?: Record<string, any>;
  onSubmit: (data: Record<string, any>, mode: "save" | "save_and_add") => void;
  isLoading?: boolean;
}

export function DataForm({
  columns,
  initialData,
  onSubmit,
  isLoading,
}: DataFormProps) {
  const [formData, setFormData] = useState<Record<string, any>>({});
  const [nulls, setNulls] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (initialData) {
      setFormData(initialData);
      const newNulls: Record<string, boolean> = {};
      columns.forEach((col) => {
        if (initialData[col.name] === null) {
          newNulls[col.name] = true;
        }
      });
      setNulls(newNulls);
    } else {
      // Initialize defaults
      const defaults: Record<string, any> = {};
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      columns.forEach((_col) => {
        // If auto_increment, stick to empty?
        // Ideally we don't send anything for auto_increment on insert
      });
      setFormData(defaults);
    }
  }, [initialData, columns]);

  const handleChange = (colName: string, value: string) => {
    setFormData((prev) => ({ ...prev, [colName]: value }));
    if (nulls[colName]) {
      setNulls((prev) => ({ ...prev, [colName]: false }));
    }
  };

  const handleNullToggle = (colName: string, checked: boolean) => {
    setNulls((prev) => ({ ...prev, [colName]: checked }));
    if (checked) {
      // If set to null, we might want to clear the form data for visual clarity
    }
  };

  const handleSubmit = (
    e: React.FormEvent,
    mode: "save" | "save_and_add" = "save"
  ) => {
    e.preventDefault();
    const finalData: Record<string, any> = {};
    columns.forEach((col) => {
      if (nulls[col.name]) {
        finalData[col.name] = null;
      } else {
        if (formData[col.name] !== undefined) {
          finalData[col.name] = formData[col.name];
        }
      }
    });
    onSubmit(finalData, mode);
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="border border-[var(--border)] rounded-md overflow-hidden">
        <table className="w-full text-sm text-left">
          <thead className="bg-[var(--muted)]/50 border-b border-[var(--border)] text-xs uppercase text-[var(--muted-foreground)]">
            <tr>
              <th className="px-4 py-3 font-medium w-1/4">Column</th>
              <th className="px-4 py-3 font-medium w-1/6">Type</th>
              <th className="px-4 py-3 font-medium">Value</th>
              <th className="px-4 py-3 font-medium w-20 text-center">Null</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border)] bg-[var(--card)]">
            {columns.map((col) => (
              <tr key={col.name} className="hover:bg-[var(--muted)]/20">
                <td className="px-4 py-3">
                  <div className="font-medium">{col.name}</div>
                  {col.key === "PRI" && (
                    <span className="text-[10px] text-yellow-500 font-mono">
                      PRIMARY
                    </span>
                  )}
                </td>
                <td className="px-4 py-3 text-xs font-mono text-[var(--muted-foreground)]">
                  {col.type}
                </td>
                <td className="px-4 py-3">
                  {(() => {
                    const type = col.type.toLowerCase();
                    let inputType = "text";
                    if (
                      type.includes("int") ||
                      type.includes("decimal") ||
                      type.includes("float") ||
                      type.includes("double")
                    ) {
                      inputType = "number";
                    } else if (
                      type.includes("datetime") ||
                      type.includes("timestamp")
                    ) {
                      inputType = "datetime-local";
                    } else if (type.includes("date")) {
                      inputType = "date";
                    }

                    return (
                      <Input
                        type={inputType}
                        value={
                          formData[col.name] === null
                            ? ""
                            : formData[col.name] || ""
                        }
                        onChange={(e) => handleChange(col.name, e.target.value)}
                        disabled={nulls[col.name]}
                        className="h-8 font-mono text-sm w-full"
                        placeholder={
                          col.default ? `Default: ${col.default}` : ""
                        }
                        step={
                          type.includes("decimal") ||
                          type.includes("float") ||
                          type.includes("double")
                            ? "any"
                            : undefined
                        }
                      />
                    );
                  })()}
                </td>
                <td className="px-4 py-3 text-center">
                  {col.nullable && (
                    <div className="flex justify-center">
                      <Checkbox
                        checked={!!nulls[col.name]}
                        onChange={(e) =>
                          handleNullToggle(col.name, e.target.checked)
                        }
                      />
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="flex justify-end gap-3">
        <Button type="button" variant="ghost">
          Cancel
        </Button>
        <div className="flex gap-2">
          {!initialData && (
            <Button
              type="button"
              variant="secondary"
              onClick={(e) => handleSubmit(e, "save_and_add")}
              loading={isLoading}
            >
              Save & Add Another
            </Button>
          )}
          <Button
            type="button"
            onClick={(e) => handleSubmit(e, "save")}
            loading={isLoading}
          >
            {initialData ? "Update Row" : "Save"}
          </Button>
        </div>
      </div>
    </form>
  );
}
