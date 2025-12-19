import { useState, useMemo } from "react";
import {
  Search,
  FolderGit2,
  ExternalLink,
  Trash2,
  Globe,
  ShieldCheck,
  ShieldAlert,
  FolderOpen,
} from "lucide-react";
import { type Project } from "@/api/daemon";
import {
  useSites,
  useIgnoreMutation,
  useUnlinkMutation,
  useEditors,
  useOpenInEditorMutation,
} from "@/hooks/use-daemon";
import { Modal } from "@/components/ui/Modal";
import { CreateProjectModal } from "@/components/CreateProjectModal";
import { Code } from "lucide-react";

export default function Projects() {
  const { data: projects = [], isLoading } = useSites();
  const { data: editors = [] } = useEditors();
  const ignoreMutation = useIgnoreMutation();
  const unlinkMutation = useUnlinkMutation();
  const openEditorMutation = useOpenInEditorMutation();

  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<"all" | "parked" | "linked">("all");
  const [projectToRemove, setProjectToRemove] = useState<Project | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isEditorModalOpen, setIsEditorModalOpen] = useState(false);
  const [selectedProjectPath, setSelectedProjectPath] = useState<string | null>(
    null
  );

  // Determine default editor (e.g. VS Code if available)
  const defaultEditor =
    editors.find((e: any) => e.id === "vscode") || editors[0];

  const handleOpenInEditor = (path: string, editorId?: string) => {
    const id = editorId || defaultEditor?.id;
    if (id) {
      openEditorMutation.mutate({ path, editor: id });
    }
  };

  const filteredProjects = useMemo(() => {
    return projects.filter((project) => {
      const matchesSearch = project.name
        .toLowerCase()
        .includes(search.toLowerCase());
      const matchesFilter = filter === "all" || project.type === filter;
      return matchesSearch && matchesFilter;
    });
  }, [projects, search, filter]);

  const handleRemoveClick = (project: Project) => {
    setProjectToRemove(project);
  };

  const confirmRemove = async () => {
    if (!projectToRemove) return;

    if (projectToRemove.type === "linked") {
      unlinkMutation.mutate(projectToRemove.name);
    } else {
      // For parked projects, we now "ignore" them instead of unparking the parent
      ignoreMutation.mutate(projectToRemove.path);
    }

    setProjectToRemove(null);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--primary)] text-[var(--primary)]"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold bg-gradient-to-r from-[var(--primary)] to-purple-400 bg-clip-text text-transparent">
            Projects
          </h1>
          <p className="text-[var(--muted-foreground)] text-sm">
            {projects.length} projects registered
          </p>
        </div>
        <button
          onClick={() => setIsCreateModalOpen(true)}
          className="bg-[var(--primary)] hover:bg-[var(--primary)]/90 text-white px-4 py-2 rounded-lg font-medium shadow-lg shadow-[var(--primary)]/20 transition-all flex items-center gap-2"
        >
          <FolderGit2 size={18} />
          Create Project
        </button>
      </div>

      {/* Search and Filter */}
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)] w-4 h-4" />
          <input
            type="text"
            placeholder="Search projects..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 rounded-lg bg-[var(--card)] border border-[var(--border)] focus:outline-none focus:ring-2 focus:ring-[var(--primary)] focus:border-transparent transition-all"
          />
        </div>
        <div className="flex bg-[var(--card)] rounded-lg p-1 border border-[var(--border)]">
          {(["all", "parked", "linked"] as const).map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`px-4 py-1.5 rounded-md text-sm font-medium transition-all ${
                filter === f
                  ? "bg-[var(--primary)] text-white shadow-sm"
                  : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
              }`}
            >
              {f.charAt(0).toUpperCase() + f.slice(1)}
            </button>
          ))}
        </div>
      </div>

      {/* Projects Grid */}
      {filteredProjects.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-64 border-2 border-dashed border-[var(--border)] rounded-xl bg-[var(--card)]/50">
          <FolderOpen className="w-12 h-12 text-[var(--muted-foreground)] mb-4 opacity-50" />
          <p className="text-[var(--muted-foreground)] font-medium">
            No projects found
          </p>
          <p className="text-sm text-[var(--muted-foreground)]/80">
            Park a folder or link a project to get started
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
          {filteredProjects.map((project) => (
            <div
              key={project.path}
              className="group relative bg-[var(--card)] rounded-xl p-5 border border-[var(--border)] hover:border-[var(--primary)]/50 transition-all hover:shadow-[0_4px_20px_-10px_rgba(0,0,0,0.3)] hover:-translate-y-1"
            >
              <div className="flex items-start justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div
                    className={`p-2.5 rounded-lg ${
                      project.type === "parked"
                        ? "bg-blue-500/10 text-blue-500"
                        : "bg-purple-500/10 text-purple-500"
                    }`}
                  >
                    <FolderGit2 size={20} />
                  </div>
                  <div>
                    <h3 className="font-semibold text-lg leading-none mb-1.5">
                      {project.name}
                    </h3>
                    <span className="text-xs px-2 py-0.5 rounded-full bg-[var(--muted)] text-[var(--muted-foreground)] font-medium">
                      {project.type}
                    </span>
                  </div>
                </div>
                <button
                  onClick={() => handleRemoveClick(project)}
                  className="p-2 text-[var(--muted-foreground)] hover:text-red-500 hover:bg-red-500/10 rounded-lg transition-colors opacity-0 group-hover:opacity-100 focus:opacity-100"
                  title="Remove Project"
                >
                  <Trash2 size={16} />
                </button>
              </div>

              <div className="space-y-3 mb-5">
                <div className="flex items-center text-sm text-[var(--muted-foreground)] bg-[var(--background)]/50 p-2 rounded-md truncate">
                  <code className="text-xs font-mono truncate select-all">
                    {project.path}
                  </code>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <div className="flex items-center gap-2 text-[var(--muted-foreground)]">
                    <span className="font-mono text-xs bg-[var(--muted)] px-1.5 py-0.5 rounded text-[var(--foreground)]">
                      PHP {project.phpVersion}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    {project.secure ? (
                      <span className="flex items-center gap-1 text-[10px] font-bold text-emerald-500 bg-emerald-500/10 px-2 py-0.5 rounded-full uppercase tracking-wider">
                        <ShieldCheck size={10} /> Secure
                      </span>
                    ) : (
                      <span className="flex items-center gap-1 text-[10px] font-bold text-amber-500 bg-amber-500/10 px-2 py-0.5 rounded-full uppercase tracking-wider">
                        <ShieldAlert size={10} /> Unsecure
                      </span>
                    )}
                  </div>
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <a
                  href={`http://${project.domain}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-center gap-2 w-full py-2.5 bg-[var(--primary)]/10 text-[var(--primary)] hover:bg-[var(--primary)] hover:text-white rounded-lg font-medium transition-all group-hover:shadow-md"
                >
                  <Globe size={16} />
                  Visit {project.domain}
                  <ExternalLink size={14} className="opacity-50" />
                </a>

                {/* Editor Button */}
                {editors.length > 0 && (
                  <button
                    onClick={() => {
                      setSelectedProjectPath(project.path);
                      setIsEditorModalOpen(true);
                    }}
                    className="flex items-center justify-center gap-2 w-full py-2.5 bg-[var(--card)] border border-[var(--border)] hover:bg-[var(--muted)] hover:border-[var(--primary)]/30 text-[var(--foreground)] rounded-lg font-medium transition-all"
                  >
                    <Code size={16} />
                    Open in Editor
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Remove Project Modal */}
      <Modal
        isOpen={!!projectToRemove}
        onClose={() => setProjectToRemove(null)}
        title="Remove Project"
        footer={
          <div className="flex justify-end gap-3">
            <button
              onClick={() => setProjectToRemove(null)}
              className="px-4 py-2 rounded-lg text-sm font-medium hover:bg-[var(--muted)] transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={confirmRemove}
              className="px-4 py-2 rounded-lg bg-red-500 hover:bg-red-600 text-white text-sm font-medium transition-colors shadow-lg shadow-red-500/20"
            >
              Remove
            </button>
          </div>
        }
      >
        <div className="space-y-4">
          <div className="p-4 bg-red-500/10 rounded-xl flex gap-3 text-red-500 items-start">
            <ShieldAlert className="shrink-0 mt-0.5" />
            <div className="text-sm">
              <p className="font-semibold mb-1">
                Remove "{projectToRemove?.name}"?
              </p>
              {projectToRemove?.type === "parked" ? (
                <p className="opacity-90">
                  This will <strong>hide</strong> the project from the
                  dashboard. The folder and files on your disk will{" "}
                  <strong>NOT</strong> be deleted.
                </p>
              ) : (
                <p className="opacity-90">
                  This will unlink the project. The folder and files on your
                  disk will <strong>NOT</strong> be deleted.
                </p>
              )}
            </div>
          </div>
        </div>
      </Modal>

      {/* Editor Selection Modal */}
      <Modal
        isOpen={isEditorModalOpen}
        onClose={() => setIsEditorModalOpen(false)}
        title="Select Editor"
        footer={
          <div className="flex justify-end">
            <button
              onClick={() => setIsEditorModalOpen(false)}
              className="px-4 py-2 rounded-lg text-sm font-medium hover:bg-[var(--muted)] transition-colors"
            >
              Cancel
            </button>
          </div>
        }
      >
        <div className="space-y-2">
          <p className="text-sm text-[var(--muted-foreground)] mb-4">
            Select an editor to open the project with:
          </p>
          <div className="grid grid-cols-1 gap-2">
            {editors.map((editor: any) => (
              <button
                key={editor.id}
                onClick={() => {
                  if (selectedProjectPath) {
                    handleOpenInEditor(selectedProjectPath, editor.id);
                    setIsEditorModalOpen(false);
                  }
                }}
                className="flex items-center gap-3 p-3 rounded-lg border border-[var(--border)] hover:border-[var(--primary)] hover:bg-[var(--primary)]/5 transition-all text-left group"
              >
                <div className="p-2 rounded-md bg-[var(--muted)] group-hover:bg-[var(--background)] transition-colors">
                  <Code size={20} className="text-[var(--foreground)]" />
                </div>
                <div>
                  <div className="font-medium text-sm text-[var(--foreground)]">
                    {editor.name}
                  </div>
                  <div className="text-xs text-[var(--muted-foreground)] font-mono">
                    {editor.bin}
                  </div>
                </div>
              </button>
            ))}
          </div>
        </div>
      </Modal>

      <CreateProjectModal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        onCreated={() => {
          // useSites hooks automatically invalidates, so list should refresh
        }}
      />
    </div>
  );
}
