import { useState, useEffect } from "react";
import {
  FileText,
  Save,
  RotateCcw,
  Plus,
  Trash2,
  Eye,
  EyeOff,
  History,
  FileCode,
  List,
  X,
} from "lucide-react";
import { Modal } from "@/components/ui/Modal";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import {
  useEnvFiles,
  useEnvFile,
  useSaveEnvMutation,
  useEnvBackups,
  useRestoreEnvBackupMutation,
} from "@/hooks/use-daemon";

interface EnvEditorProps {
  isOpen: boolean;
  onClose: () => void;
  projectPath: string;
  projectName: string;
}

export function EnvEditor({
  isOpen,
  onClose,
  projectPath,
  projectName,
}: EnvEditorProps) {
  const [selectedFile, setSelectedFile] = useState<string>("");
  const [activeTab, setActiveTab] = useState("editor");
  const [showSensitive, setShowSensitive] = useState(false);

  const { data: envFiles = [], isLoading: isLoadingFiles } = useEnvFiles(
    isOpen ? projectPath : undefined
  );

  // Select first file by default when files are loaded
  useEffect(() => {
    if (envFiles.length > 0 && !selectedFile) {
      // Prefer .env over others
      const mainEnv = envFiles.find((f) => f.name === ".env");
      setSelectedFile(mainEnv ? mainEnv.path : envFiles[0].path);
    }
  }, [envFiles, selectedFile]);

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={`Environment Variables - ${projectName}`}
      className="max-w-5xl"
    >
      <div className="flex flex-col h-[600px] -mx-6 -my-4">
        <div className="flex flex-1 overflow-hidden">
          {/* Sidebar - File List */}
          <div className="w-48 border-r border-[var(--border)] bg-[var(--muted)]/30 p-4 flex flex-col gap-2">
            <h3 className="text-xs font-bold text-[var(--muted-foreground)] uppercase tracking-wider mb-2">
              Files
            </h3>
            {isLoadingFiles ? (
              <div className="space-y-2 animate-pulse">
                <div className="h-8 bg-[var(--muted)] rounded-md"></div>
                <div className="h-8 bg-[var(--muted)] rounded-md"></div>
              </div>
            ) : envFiles.length === 0 ? (
              <p className="text-sm text-[var(--muted-foreground)]">
                No .env files found
              </p>
            ) : (
              envFiles.map((file) => (
                <button
                  key={file.path}
                  onClick={() => setSelectedFile(file.path)}
                  className={`flex items-center gap-2 px-3 py-2 rounded-md text-sm transition-colors ${
                    selectedFile === file.path
                      ? "bg-[var(--primary)] text-white shadow-sm"
                      : "hover:bg-[var(--muted)] text-[var(--foreground)]"
                  }`}
                >
                  <FileText size={14} />
                  {file.name}
                </button>
              ))
            )}
          </div>

          {/* Main Content */}
          <div className="flex-1 flex flex-col overflow-hidden bg-[var(--card)]">
            {selectedFile && (
              <FileEditor
                path={selectedFile}
                activeTab={activeTab}
                setActiveTab={setActiveTab}
                showSensitive={showSensitive}
                setShowSensitive={setShowSensitive}
                onClose={onClose}
              />
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
}

function FileEditor({
  path,
  activeTab,
  setActiveTab,
  showSensitive,
  setShowSensitive,
  onClose,
}: {
  path: string;
  activeTab: string;
  setActiveTab: (t: string) => void;
  showSensitive: boolean;
  setShowSensitive: (v: boolean) => void;
  onClose: () => void;
}) {
  const { data: envFile, isLoading } = useEnvFile(path);
  const saveMutation = useSaveEnvMutation();

  const [variables, setVariables] = useState<{ key: string; value: string }[]>(
    []
  );
  const [rawContent, setRawContent] = useState("");
  const [isDirty, setIsDirty] = useState(false);

  // Parse file content when loaded
  useEffect(() => {
    if (envFile) {
      // Convert object to array for editor
      const vars = Object.entries(envFile.variables).map(([key, value]) => ({
        key,
        value,
      }));
      // Sort by key
      vars.sort((a, b) => a.key.localeCompare(b.key));
      setVariables(vars);

      // Also reconstruct raw content (approximate)
      const raw = vars.map((v) => `${v.key}=${v.value}`).join("\n");
      setRawContent(raw);

      setIsDirty(false);
    }
  }, [envFile]);

  const handleSave = () => {
    // Convert back based on active tab
    const newVars: Record<string, string> = {};

    if (activeTab === "raw") {
      // Parse raw content
      rawContent.split("\n").forEach((line) => {
        const parts = line.split("=");
        if (parts.length >= 2) {
          const key = parts[0].trim();
          // Join the rest back in case value contains =
          const value = parts.slice(1).join("=").trim();
          if (key && !key.startsWith("#")) {
            newVars[key] = value;
          }
        }
      });
    } else {
      variables.forEach((v) => {
        if (v.key) newVars[v.key] = v.value;
      });
    }

    saveMutation.mutate({ path, variables: newVars });
    setIsDirty(false);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--primary)] text-[var(--primary)]"></div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="p-4 border-b border-[var(--border)] flex items-center justify-between">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList>
            <TabsTrigger value="editor">
              <List size={14} className="mr-2" /> Editor
            </TabsTrigger>
            <TabsTrigger value="raw">
              <FileCode size={14} className="mr-2" /> Raw
            </TabsTrigger>
            <TabsTrigger value="backups">
              <History size={14} className="mr-2" /> Backups
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="flex items-center gap-2">
          {activeTab !== "backups" && (
            <button
              onClick={() => setShowSensitive(!showSensitive)}
              className="p-2 text-[var(--muted-foreground)] hover:text-[var(--foreground)] rounded-md transition-colors"
              title={
                showSensitive
                  ? "Hide sensitive values"
                  : "Show sensitive values"
              }
            >
              {showSensitive ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          )}

          {activeTab !== "backups" && (
            <Button
              variant="ghost"
              onClick={onClose}
              loading={saveMutation.isPending}
              className="flex items-center gap-2"
            >
              <X size={16} />
              Cancel
            </Button>
          )}

          {activeTab !== "backups" && (
            <Button
              onClick={handleSave}
              loading={saveMutation.isPending}
              disabled={!isDirty || saveMutation.isPending}
              className="gap-2"
            >
              <Save size={16} />
              Save Changes
            </Button>
          )}
        </div>
      </div>

      {/* Editor Content */}
      <div className="flex-1 overflow-auto p-4">
        {activeTab === "editor" && (
          <div className="space-y-2">
            {variables.map((v, idx) => (
              <div key={idx} className="flex gap-2 group">
                <Input
                  value={v.key}
                  onChange={(e) => {
                    const newVars = [...variables];
                    newVars[idx].key = e.target.value;
                    setVariables(newVars);
                    setIsDirty(true);
                  }}
                  placeholder="KEY"
                  className="w-1/3 font-mono text-sm"
                />
                <div className="relative flex-1">
                  <Input
                    type={
                      !showSensitive && isSensitiveKey(v.key)
                        ? "password"
                        : "text"
                    }
                    value={v.value}
                    onChange={(e) => {
                      const newVars = [...variables];
                      newVars[idx].value = e.target.value;
                      setVariables(newVars);
                      setIsDirty(true);
                    }}
                    placeholder="Value"
                    className="w-full font-mono text-sm"
                  />
                </div>
                <button
                  onClick={() => {
                    const newVars = variables.filter((_, i) => i !== idx);
                    setVariables(newVars);
                    setIsDirty(true);
                  }}
                  className="p-2 text-[var(--muted-foreground)] hover:text-red-500 rounded-md opacity-0 group-hover:opacity-100 transition-all"
                >
                  <Trash2 size={16} />
                </button>
              </div>
            ))}

            <button
              onClick={() => {
                setVariables([...variables, { key: "", value: "" }]);
                setIsDirty(true);
              }}
              className="w-full py-2 border border-dashed border-[var(--border)] rounded-md text-[var(--muted-foreground)] hover:text-[var(--primary)] hover:border-[var(--primary)] hover:bg-[var(--primary)]/5 transition-all flex items-center justify-center gap-2 text-sm mt-4"
            >
              <Plus size={16} />
              Add Variable
            </button>
          </div>
        )}

        {activeTab === "raw" && (
          <textarea
            value={rawContent}
            onChange={(e) => {
              setRawContent(e.target.value);
              setIsDirty(true);
            }}
            className="w-full h-full bg-[var(--background)] p-4 rounded-md font-mono text-sm border border-[var(--border)] focus:outline-none focus:ring-2 focus:ring-[var(--primary)] resize-none"
            spellCheck={false}
          />
        )}

        {activeTab === "backups" && <BackupList path={path} />}
      </div>
    </div>
  );
}

function BackupList({ path }: { path: string }) {
  const { data: backups = [], isLoading } = useEnvBackups(path);
  const restoreMutation = useRestoreEnvBackupMutation();

  if (isLoading)
    return <div className="text-center p-4">Loading backups...</div>;
  if (backups.length === 0)
    return (
      <div className="text-center p-8 text-[var(--muted-foreground)]">
        No backups found
      </div>
    );

  return (
    <div className="space-y-3">
      {backups.map((backup) => (
        <div
          key={backup.filename}
          className="flex items-center justify-between p-3 bg-[var(--background)] rounded-lg border border-[var(--border)]"
        >
          <div className="flex items-center gap-3">
            <div className="p-2 bg-[var(--muted)] rounded-md text-[var(--muted-foreground)]">
              <History size={18} />
            </div>
            <div>
              <div className="font-medium text-sm">{backup.filename}</div>
              <div className="text-xs text-[var(--muted-foreground)]">
                {new Date(backup.created_at).toLocaleString()} •{" "}
                {formatBytes(backup.size)}
              </div>
            </div>
          </div>
          <Button
            size="sm"
            variant="secondary"
            className="gap-2"
            onClick={() => {
              if (
                confirm(
                  "Are you sure you want to restore this backup? Current file will be backed up."
                )
              ) {
                restoreMutation.mutate({
                  backupPath: backup.path,
                  targetPath: path,
                });
              }
            }}
            loading={restoreMutation.isPending}
          >
            <RotateCcw size={14} />
            Restore
          </Button>
        </div>
      ))}
    </div>
  );
}

// Helpers
function isSensitiveKey(key: string): boolean {
  const sensitive = [
    "KEY",
    "SECRET",
    "PASSWORD",
    "TOKEN",
    "AUTH",
    "CREDENTIAL",
    "PRIVATE",
    "CRYPT",
  ];
  const upper = key.toUpperCase();
  return sensitive.some((s) => upper.includes(s));
}

function formatBytes(bytes: number) {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}
