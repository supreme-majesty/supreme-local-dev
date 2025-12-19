import { useState } from "react";
import { Modal } from "@/components/ui/Modal";
import { type ProjectOptions, api } from "@/api/daemon";
import { Loader2, Plus, Folder } from "lucide-react";
import { useDirectories } from "@/hooks/use-daemon";

interface CreateProjectModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export function CreateProjectModal({
  isOpen,
  onClose,
  onCreated,
}: CreateProjectModalProps) {
  const [name, setName] = useState("");
  const [type, setType] = useState<ProjectOptions["type"]>("laravel");
  const [targetDir, setTargetDir] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Directory navigation
  const { data: directories = [] } = useDirectories(targetDir || undefined);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name) return;

    setIsLoading(true);
    setError(null);

    try {
      await api.createProject({ type, name, directory: targetDir });
      onCreated();
      onClose();
      setName("");
      setType("laravel");
    } catch (err: any) {
      setError(err.message || "Failed to create project");
    } finally {
      setIsLoading(false);
    }
  };

  const projectTypes = [
    { value: "laravel", label: "Laravel (Composer)" },
    { value: "react", label: "React (Vite)" },
    { value: "vue", label: "Vue (Vite)" },
    { value: "nextjs", label: "Next.js" },
    { value: "nodejs", label: "Node.js (Basic)" },
  ];

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Create New Project"
      footer={
        <div className="flex justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-lg text-sm font-medium hover:bg-[var(--muted)] transition-colors"
            disabled={isLoading}
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={isLoading || !name}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-[var(--primary)] hover:bg-[var(--primary)]/90 text-white text-sm font-medium transition-colors shadow-lg shadow-[var(--primary)]/20 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isLoading ? (
              <>
                <Loader2 className="animate-spin w-4 h-4" /> Creating...
              </>
            ) : (
              <>
                <Plus className="w-4 h-4" /> Create Project
              </>
            )}
          </button>
        </div>
      }
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        {error && (
          <div className="p-3 bg-red-500/10 text-red-500 text-sm rounded-lg border border-red-500/20">
            {error}
          </div>
        )}

        <div className="space-y-2">
          <label className="text-sm font-medium text-[var(--foreground)]">
            Project Name
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value.replace(/\s+/g, "-"))}
            placeholder="my-awesome-project"
            className="w-full px-3 py-2 rounded-lg bg-[var(--background)] border border-[var(--border)] focus:outline-none focus:ring-2 focus:ring-[var(--primary)] text-sm font-mono"
            autoFocus
          />
          <p className="text-xs text-[var(--muted-foreground)]">
            This will be the folder name. Use hyphens instead of spaces.
          </p>
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium text-[var(--foreground)]">
            Parent Directory (Optional)
          </label>
          <div className="flex gap-2">
            <input
              type="text"
              value={targetDir}
              onChange={(e) => setTargetDir(e.target.value)}
              placeholder="Leave empty for default directory"
              className="flex-1 px-3 py-2 rounded-lg bg-[var(--background)] border border-[var(--border)] focus:outline-none focus:ring-2 focus:ring-[var(--primary)] text-sm font-mono"
            />
          </div>
          {/* Simple Directory Suggestions */}
          {/* Simple Directory Suggestions */}
          <div className="flex flex-wrap gap-2 mt-2">
            {(targetDir || "").length > 0 && (
              <button
                type="button"
                onClick={() => {
                  // Go up one level
                  if (!targetDir) return;
                  const isAbsolute = targetDir.startsWith("/");
                  const parts = targetDir.split("/").filter(Boolean);
                  parts.pop();
                  const newPath = parts.join("/");
                  setTargetDir(isAbsolute ? "/" + newPath : newPath);
                }}
                className="flex items-center gap-1 text-xs px-2 py-1 bg-[var(--muted)] hover:bg-[var(--muted)]/80 rounded border border-[var(--border)] font-mono"
                title="Go to parent directory"
              >
                <Folder className="w-3 h-3 rotate-90" /> ..
              </button>
            )}

            {/* Quick Root Access if likely at root or needed */}
            <button
              type="button"
              onClick={() => setTargetDir("/")}
              className="flex items-center gap-1 text-xs px-2 py-1 bg-[var(--muted)] hover:bg-[var(--muted)]/80 rounded border border-[var(--border)] font-mono opacity-50 hover:opacity-100"
              title="Go to System Root"
            >
              /
            </button>

            {directories.slice(0, 10).map((dir: string) => (
              <button
                key={dir}
                type="button"
                onClick={() => {
                  // Correctly join path
                  const cleanCurrent = targetDir?.endsWith("/")
                    ? targetDir.slice(0, -1)
                    : targetDir;
                  setTargetDir(cleanCurrent ? `${cleanCurrent}/${dir}` : dir);
                }}
                className="flex items-center gap-1 text-xs px-2 py-1 bg-[var(--muted)] hover:bg-[var(--muted)]/80 rounded"
              >
                <Folder size={10} /> {dir}
              </button>
            ))}
          </div>
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium text-[var(--foreground)]">
            Project Type
          </label>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {projectTypes.map((t) => (
              <button
                key={t.value}
                type="button"
                onClick={() => setType(t.value as any)}
                className={`flex items-center px-3 py-2.5 rounded-lg border text-sm transition-all text-left ${
                  type === t.value
                    ? "border-[var(--primary)] bg-[var(--primary)]/5 ring-1 ring-[var(--primary)]"
                    : "border-[var(--border)] hover:border-[var(--primary)]/50 hover:bg-[var(--muted)]/50"
                }`}
              >
                <div
                  className={`w-2 h-2 rounded-full mr-2 ${
                    type === t.value
                      ? "bg-[var(--primary)]"
                      : "bg-[var(--muted-foreground)]/30"
                  }`}
                />
                {t.label}
              </button>
            ))}
          </div>
        </div>
      </form>
    </Modal>
  );
}
