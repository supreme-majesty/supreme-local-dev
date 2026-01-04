import { useState, useEffect } from "react";
import { Modal } from "@/components/ui/Modal";

import {
  Loader2,
  Plus,
  Folder,
  LayoutGrid,
  HardDrive,
  Search,
} from "lucide-react";
import {
  useDirectories,
  useSldState,
  useCreateProjectMutation,
  useTemplates,
} from "@/hooks/use-daemon";

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
  const [type, setType] = useState("laravel");
  const [repository, setRepository] = useState("");

  const { data: templates = [] } = useTemplates();

  // Location Management
  const [targetDir, setTargetDir] = useState("");
  const [locationMode, setLocationMode] = useState<"parked" | "custom">(
    "parked"
  );
  const [isBrowserOpen, setIsBrowserOpen] = useState(false);

  const { data: state } = useSldState();
  const parkedPaths = state?.paths || [];

  const createProjectMutation = useCreateProjectMutation();
  const error = createProjectMutation.error?.message;
  const isLoading = createProjectMutation.isPending;

  // Initialize Default Location
  useEffect(() => {
    if (isOpen) {
      if (parkedPaths.length > 0 && locationMode === "parked") {
        // Only set if not already set to a valid parked path
        if (!parkedPaths.includes(targetDir)) {
          setTargetDir(parkedPaths[0]);
        }
      } else if (parkedPaths.length === 0 && locationMode === "parked") {
        // No parked paths, switch to custom
        setLocationMode("custom");
      }
    }
  }, [isOpen, parkedPaths, locationMode, targetDir]);

  // Directory navigation (custom mode)
  // Fetch directories only if browser is open and in custom mode
  const { data: directories = [] } = useDirectories(
    locationMode === "custom" && isBrowserOpen
      ? targetDir || undefined
      : undefined
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name) return;

    createProjectMutation.mutate(
      {
        type,
        name,
        directory: targetDir,
        repository: type === "custom" ? repository : undefined,
      },
      {
        onSuccess: () => {
          onCreated();
          onClose();
          setName("");
          setType("laravel");
          // Reset location to default
          if (parkedPaths.length > 0) {
            setLocationMode("parked");
            setTargetDir(parkedPaths[0]);
          } else {
            setTargetDir("");
          }
          setIsBrowserOpen(false);
        },
      }
    );
  };

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
            disabled={
              isLoading || !name || (locationMode === "parked" && !targetDir)
            }
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

        {/* Project Name */}
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

        {/* Location Selection */}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <label className="text-sm font-medium text-[var(--foreground)]">
              Installation Location
            </label>
          </div>

          {/* Mode Switch */}
          <div className="flex gap-1 p-1 bg-[var(--muted)]/50 rounded-lg w-full sm:w-fit">
            <button
              type="button"
              onClick={() => {
                setLocationMode("parked");
                if (parkedPaths.length > 0) setTargetDir(parkedPaths[0]);
                setIsBrowserOpen(false);
              }}
              className={`flex-1 sm:flex-none px-3 py-1.5 text-xs font-medium rounded-md transition-all flex items-center justify-center gap-2 ${
                locationMode === "parked"
                  ? "bg-[var(--background)] shadow-sm text-[var(--foreground)] ring-1 ring-black/5 dark:ring-white/10"
                  : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
              }`}
              disabled={parkedPaths.length === 0}
              title={
                parkedPaths.length === 0
                  ? "No parked folders found"
                  : "Install in a parked folder"
              }
            >
              <LayoutGrid size={14} /> Parked Folder
            </button>
            <button
              type="button"
              onClick={() => setLocationMode("custom")}
              className={`flex-1 sm:flex-none px-3 py-1.5 text-xs font-medium rounded-md transition-all flex items-center justify-center gap-2 ${
                locationMode === "custom"
                  ? "bg-[var(--background)] shadow-sm text-[var(--foreground)] ring-1 ring-black/5 dark:ring-white/10"
                  : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
              }`}
            >
              <HardDrive size={14} /> Custom Path
            </button>
          </div>

          {/* Parked Mode UI */}
          {locationMode === "parked" && (
            <div className="relative">
              <Folder className="absolute left-3 top-2.5 w-4 h-4 text-[var(--muted-foreground)]" />
              <select
                value={targetDir}
                onChange={(e) => setTargetDir(e.target.value)}
                className="w-full pl-9 pr-8 py-2 rounded-lg bg-[var(--background)] border border-[var(--border)] text-sm appearance-none focus:outline-none focus:ring-2 focus:ring-[var(--primary)] text-[var(--foreground)]"
              >
                {parkedPaths.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>
              <div className="absolute right-3 top-3 pointer-events-none">
                <svg
                  className="w-4 h-4 text-[var(--muted-foreground)]"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 9l-7 7-7-7"
                  />
                </svg>
              </div>
            </div>
          )}

          {/* Custom Mode UI */}
          {locationMode === "custom" && (
            <div className="space-y-2 animate-in fade-in zoom-in-95 duration-200">
              <div className="flex gap-2">
                <input
                  type="text"
                  value={targetDir}
                  onChange={(e) => setTargetDir(e.target.value)}
                  placeholder="/path/to/project"
                  className="flex-1 px-3 py-2 rounded-lg bg-[var(--background)] border border-[var(--border)] focus:outline-none focus:ring-2 focus:ring-[var(--primary)] text-sm font-mono"
                />
                <button
                  type="button"
                  onClick={() => setIsBrowserOpen(!isBrowserOpen)}
                  className={`px-3 py-2 rounded-lg border flex items-center gap-2 text-sm font-medium transition-colors ${
                    isBrowserOpen
                      ? "bg-[var(--muted)] border-[var(--border)]"
                      : "bg-[var(--background)] border-[var(--border)] hover:bg-[var(--muted)]"
                  }`}
                >
                  <Search className="w-4 h-4" />{" "}
                  {isBrowserOpen ? "Hide" : "Browse"}
                </button>
              </div>

              {/* Directory Browser - Conditional */}
              {isBrowserOpen && (
                <div className="p-3 rounded-lg border border-[var(--border)] bg-[var(--muted)]/20 animate-in slide-in-from-top-2 duration-200">
                  <div className="flex flex-wrap gap-2">
                    {(targetDir || "").length > 0 && (
                      <button
                        type="button"
                        onClick={() => {
                          if (!targetDir) return;
                          const isAbsolute = targetDir.startsWith("/");
                          const parts = targetDir.split("/").filter(Boolean);
                          parts.pop();
                          const newPath = parts.join("/");
                          setTargetDir(isAbsolute ? "/" + newPath : newPath);
                        }}
                        className="flex items-center gap-1 text-xs px-2 py-1 bg-[var(--background)] hover:bg-[var(--background)]/80 rounded border border-[var(--border)] font-mono shadow-sm"
                        title="Go to parent directory"
                      >
                        <Folder className="w-3 h-3 rotate-90" /> ..
                      </button>
                    )}

                    <button
                      type="button"
                      onClick={() => setTargetDir("/")}
                      className="flex items-center gap-1 text-xs px-2 py-1 bg-[var(--background)] hover:bg-[var(--background)]/80 rounded border border-[var(--border)] font-mono shadow-sm opacity-70 hover:opacity-100"
                      title="Go to System Root"
                    >
                      /
                    </button>

                    {directories.map((dir: string) => (
                      <button
                        key={dir}
                        type="button"
                        onClick={() => {
                          const cleanCurrent = targetDir?.endsWith("/")
                            ? targetDir.slice(0, -1)
                            : targetDir;
                          setTargetDir(
                            cleanCurrent ? `${cleanCurrent}/${dir}` : dir
                          );
                        }}
                        className="flex items-center gap-1 text-xs px-2 py-1 bg-[var(--background)] hover:bg-[var(--background)]/80 rounded border border-[var(--border)] shadow-sm"
                      >
                        <Folder size={10} /> {dir}
                      </button>
                    ))}

                    {directories.length === 0 && (
                      <span className="text-xs text-[var(--muted-foreground)] italic">
                        No subdirectories found
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Project Type */}
        <div className="space-y-2">
          <label className="text-sm font-medium text-[var(--foreground)]">
            Project Type
          </label>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {templates.map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => setType(t.id)}
                className={`flex items-center px-3 py-2.5 rounded-lg border text-sm transition-all text-left ${
                  type === t.id
                    ? "border-[var(--primary)] bg-[var(--primary)]/5 ring-1 ring-[var(--primary)]"
                    : "border-[var(--border)] hover:border-[var(--primary)]/50 hover:bg-[var(--muted)]/50"
                }`}
              >
                <div
                  className={`w-2 h-2 rounded-full mr-2 ${
                    type === t.id
                      ? "bg-[var(--primary)]"
                      : "bg-[var(--muted-foreground)]/30"
                  }`}
                />
                <div className="flex flex-col">
                  <span>{t.name}</span>
                  <span className="text-[10px] text-[var(--muted-foreground)] line-clamp-1">
                    {t.description}
                  </span>
                </div>
              </button>
            ))}
          </div>
        </div>

        {/* Repository Input (Custom) */}
        {type === "custom" && (
          <div className="space-y-2 animate-in fade-in zoom-in-95 duration-200">
            <label className="text-sm font-medium text-[var(--foreground)]">
              Git Repository URL
            </label>
            <input
              type="text"
              value={repository}
              onChange={(e) => setRepository(e.target.value)}
              placeholder="https://github.com/username/repo.git"
              className="w-full px-3 py-2 rounded-lg bg-[var(--background)] border border-[var(--border)] focus:outline-none focus:ring-2 focus:ring-[var(--primary)] text-sm font-mono"
            />
          </div>
        )}
      </form>
    </Modal>
  );
}
