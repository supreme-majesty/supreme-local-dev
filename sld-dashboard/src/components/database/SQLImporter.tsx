import { useState, useRef } from "react";
import {
  FileUp,
  Upload,
  CheckCircle2,
  AlertCircle,
  Database,
  FileCode2,
  X,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useImportDatabaseMutation } from "@/hooks/use-database";
import { formatBytes } from "@/lib/utils";

interface SQLImporterProps {
  database: string;
  serverLabel?: string;
  onSuccessViewTables?: () => void;
}

const MAX_SIZE_BYTES = 100 * 1024 * 1024; // 100MB

export function SQLImporter({
  database,
  serverLabel,
  onSuccessViewTables,
}: SQLImporterProps) {
  const importMutation = useImportDatabaseMutation();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  const pickFile = (file: File | undefined) => {
    setValidationError(null);
    setSuccessMessage(null);
    importMutation.reset();

    if (!file) return;

    const lower = file.name.toLowerCase();
    if (!lower.endsWith(".sql") && !lower.endsWith(".sql.gz") && !lower.endsWith(".gz")) {
      setValidationError("Please choose a .sql or .sql.gz file.");
      setSelectedFile(null);
      return;
    }
    if (file.size > MAX_SIZE_BYTES) {
      setValidationError("File exceeds the 100 MB upload limit.");
      setSelectedFile(null);
      return;
    }
    setSelectedFile(file);
  };

  const handleImport = () => {
    if (!selectedFile) return;
    setValidationError(null);
    setSuccessMessage(null);

    importMutation.mutate(
      { file: selectedFile, database, restore: true },
      {
        onSuccess: (data) => {
          setSuccessMessage(
            data?.message || `Imported ${selectedFile.name} into ${database}.`,
          );
          setSelectedFile(null);
          if (fileInputRef.current) {
            fileInputRef.current.value = "";
          }
        },
      },
    );
  };

  const errorMessage =
    validationError ||
    (importMutation.isError && importMutation.error instanceof Error
      ? importMutation.error.message
      : null);

  return (
    <div className="max-w-3xl mx-auto space-y-6">
      <Card className="border-[var(--border)]">
        <CardHeader>
          <CardTitle className="text-lg flex items-center gap-2">
            <FileUp size={18} className="text-[var(--primary)]" />
            Import SQL Dump into <span className="font-mono text-[var(--primary)]">{database}</span>
          </CardTitle>
          <CardDescription>
            Upload a <code className="font-mono">.sql</code> or{" "}
            <code className="font-mono">.sql.gz</code> file (up to 100 MB) to execute directly against{" "}
            <strong>{serverLabel || "the active database server"}</strong>.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Dropzone */}
          <div
            onClick={() => !importMutation.isPending && fileInputRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault();
              setIsDragging(true);
            }}
            onDragLeave={() => setIsDragging(false)}
            onDrop={(e) => {
              e.preventDefault();
              setIsDragging(false);
              pickFile(e.dataTransfer.files?.[0]);
            }}
            className={`border-2 border-dashed rounded-xl p-10 text-center transition-colors ${
              isDragging
                ? "border-[var(--primary)] bg-[var(--primary)]/10"
                : "border-[var(--border)] hover:border-[var(--primary)]/60 bg-[var(--muted)]/10"
            } ${importMutation.isPending ? "opacity-60 pointer-events-none" : "cursor-pointer"}`}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".sql,.gz,.sql.gz"
              className="hidden"
              onChange={(e) => pickFile(e.target.files?.[0])}
            />
            <div className="mx-auto w-12 h-12 rounded-full bg-[var(--primary)]/10 text-[var(--primary)] flex items-center justify-center mb-3">
              <Upload size={22} />
            </div>
            <p className="text-sm font-medium mb-1">
              Drop a <code className="font-mono">.sql</code> or <code className="font-mono">.sql.gz</code> file here, or click to browse
            </p>
            <p className="text-xs text-[var(--muted-foreground)]">
              Supports full mysqldump / pg_dump exports, CREATE TABLE, INSERT, and gzip-compressed dumps
            </p>
          </div>

          {/* Selected file preview */}
          {selectedFile && (
            <div className="flex items-center justify-between gap-3 p-3 rounded-lg border border-[var(--border)] bg-[var(--card)]">
              <div className="flex items-center gap-3 min-w-0">
                <FileCode2 size={20} className="text-[var(--primary)] shrink-0" />
                <div className="min-w-0">
                  <div className="text-sm font-medium truncate">{selectedFile.name}</div>
                  <div className="text-xs text-[var(--muted-foreground)]">
                    {formatBytes(selectedFile.size)} → <span className="font-mono">{database}</span>
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={importMutation.isPending}
                  onClick={() => {
                    setSelectedFile(null);
                    if (fileInputRef.current) fileInputRef.current.value = "";
                  }}
                >
                  <X size={14} />
                </Button>
                <Button
                  onClick={handleImport}
                  loading={importMutation.isPending}
                  disabled={importMutation.isPending}
                >
                  <Database size={14} className="mr-1.5" />
                  {importMutation.isPending ? "Importing..." : "Run SQL Import"}
                </Button>
              </div>
            </div>
          )}

          {/* Error banner */}
          {errorMessage && (
            <div className="p-3 rounded-lg border border-red-500/30 bg-red-500/10 text-red-400 text-xs flex items-start gap-2">
              <AlertCircle size={16} className="shrink-0 mt-0.5" />
              <div className="font-mono whitespace-pre-wrap break-all">{errorMessage}</div>
            </div>
          )}

          {/* Success banner */}
          {successMessage && (
            <div className="p-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div className="flex items-center gap-2 text-emerald-400 text-sm">
                <CheckCircle2 size={18} className="shrink-0" />
                <span>{successMessage}</span>
              </div>
              {onSuccessViewTables && (
                <Button variant="secondary" size="sm" onClick={onSuccessViewTables}>
                  View Tables
                </Button>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

