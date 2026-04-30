import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * Merge Tailwind CSS classes with proper precedence
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Format a file path for display (show last N parts)
 */
export function formatPath(path: string, parts: number = 2): string {
  const segments = path.split("/").filter(Boolean);
  return segments.slice(-parts).join("/");
}

/**
 * Format bytes to human readable size
 */
export function formatBytes(bytes: number): string {
  const units = ["B", "KB", "MB", "GB"];
  let unitIndex = 0;
  let value = bytes;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }

  return `${value.toFixed(1)} ${units[unitIndex]}`;
}

/**
 * Debounce function
 */
export function debounce<T extends (...args: Parameters<T>) => void>(
  fn: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timeoutId: ReturnType<typeof setTimeout>;
  return (...args: Parameters<T>) => {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn(...args), delay);
  };
}

/**
 * Generate a domain URL from project name
 */
export function getProjectUrl(
  name: string,
  tld: string = "test",
  secure: boolean = false
): string {
  const protocol = secure ? "https" : "http";
  return `${protocol}://${name}.${tld}`;
}

/**
 * Format a date string
 */
export function formatDate(dateStr: string): string {
  if (!dateStr) return "";
  try {
    return new Date(dateStr).toLocaleString("en-US", {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "numeric",
    });
  } catch {
    return dateStr;
  }
}

/**
 * Convert rows to CSV string
 */
export function convertToCSV(columns: string[], rows: Record<string, unknown>[]): string {
  const header = columns.join(",");
  const body = rows
    .map((row) =>
      columns
        .map((col) => {
          const val = row[col];
          if (val === null || val === undefined) return "";
          const str = String(val);
          return str.includes(",") || str.includes("\"") ? `"${str.replace(/"/g, '""')}"` : str;
        })
        .join(",")
    )
    .join("\n");
  return `${header}\n${body}`;
}

/**
 * Convert rows to SQL INSERT statements
 */
export function convertToSQL(tableName: string, columns: string[], rows: Record<string, unknown>[]): string {
  if (rows.length === 0) return "";
  
  const escape = (val: unknown) => {
    if (val === null || val === undefined) return "NULL";
    if (typeof val === "number") return val;
    return `'${String(val).replace(/'/g, "''")}'`;
  };

  const chunks = rows.map(row => {
    const vals = columns.map(col => escape(row[col])).join(", ");
    return `INSERT INTO \`${tableName}\` (\`${columns.join("`, `")}\`) VALUES (${vals});`;
  });
  
  return chunks.join("\n");
}

/**
 * Copy text to clipboard with feedback support
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (err) {
    console.error("Failed to copy text: ", err);
    return false;
  }
}
