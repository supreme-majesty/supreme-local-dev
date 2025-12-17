import { useState, useMemo } from "react";
import {
  FolderOpen,
  Search,
  ExternalLink,
  Lock,
  LockOpen,
  MoreVertical,
  Trash2,
  FolderPlus,
  Link,
} from "lucide-react";
import { Card, CardHeader } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useAppStore } from "@/stores/useAppStore";
import { cn } from "@/lib/utils";

export default function Projects() {
  const { projects, state, forgetPath, unlinkProject } = useAppStore();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<"all" | "parked" | "linked">("all");

  const filteredProjects = useMemo(() => {
    return projects.filter((project) => {
      const matchesSearch =
        project.name.toLowerCase().includes(search.toLowerCase()) ||
        project.path.toLowerCase().includes(search.toLowerCase());
      const matchesFilter = filter === "all" || project.type === filter;
      return matchesSearch && matchesFilter;
    });
  }, [projects, search, filter]);

  const handleOpenUrl = (domain: string, secure: boolean) => {
    const protocol = secure ? "https" : "http";
    window.open(`${protocol}://${domain}`, "_blank");
  };

  const handleRemoveProject = async (project: (typeof projects)[0]) => {
    if (!confirm(`Remove ${project.name}?`)) return;

    if (project.type === "linked") {
      await unlinkProject(project.name);
    } else {
      await forgetPath(project.path);
    }
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Projects</h1>
          <p className="text-[var(--muted-foreground)]">
            {projects.length} projects registered
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary">
            <FolderPlus size={16} />
            Park Folder
          </Button>
          <Button variant="primary">
            <Link size={16} />
            Link Project
          </Button>
        </div>
      </div>

      {/* Filters */}
      <Card hover={false}>
        <div className="flex flex-col sm:flex-row gap-4">
          {/* Search */}
          <div className="relative flex-1">
            <Search
              size={18}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]"
            />
            <input
              type="text"
              placeholder="Search projects..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className={cn(
                "w-full pl-10 pr-4 py-2 rounded-lg",
                "bg-[var(--input)] border border-[var(--border)]",
                "text-[var(--foreground)] placeholder-[var(--muted-foreground)]",
                "focus:outline-none focus:border-[var(--ring)]",
                "transition-colors duration-200"
              )}
            />
          </div>

          {/* Filter Tabs */}
          <div className="flex gap-1 p-1 bg-[var(--muted)]/30 rounded-lg">
            {(["all", "parked", "linked"] as const).map((f) => (
              <button
                key={f}
                onClick={() => setFilter(f)}
                className={cn(
                  "px-4 py-1.5 text-sm font-medium rounded-md transition-all duration-200",
                  filter === f
                    ? "bg-[var(--primary)] text-[var(--primary-foreground)]"
                    : "text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                )}
              >
                {f.charAt(0).toUpperCase() + f.slice(1)}
              </button>
            ))}
          </div>
        </div>
      </Card>

      {/* Projects Table */}
      <Card hover={false}>
        {filteredProjects.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <th className="text-left p-3 text-sm font-medium text-[var(--muted-foreground)]">
                    Name
                  </th>
                  <th className="text-left p-3 text-sm font-medium text-[var(--muted-foreground)]">
                    Domain
                  </th>
                  <th className="text-left p-3 text-sm font-medium text-[var(--muted-foreground)]">
                    Type
                  </th>
                  <th className="text-left p-3 text-sm font-medium text-[var(--muted-foreground)]">
                    HTTPS
                  </th>
                  <th className="text-right p-3 text-sm font-medium text-[var(--muted-foreground)]">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {filteredProjects.map((project) => (
                  <tr
                    key={project.name + project.path}
                    className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--muted)]/20 transition-colors"
                  >
                    <td className="p-3">
                      <div className="flex items-center gap-3">
                        <FolderOpen
                          size={18}
                          className="text-[var(--muted-foreground)]"
                        />
                        <div>
                          <p className="font-medium text-[var(--foreground)]">
                            {project.name}
                          </p>
                          <p className="text-xs text-[var(--muted-foreground)] font-mono truncate max-w-[200px]">
                            {project.path}
                          </p>
                        </div>
                      </div>
                    </td>
                    <td className="p-3">
                      <code className="text-sm font-mono text-[var(--primary)]">
                        {project.domain}
                      </code>
                    </td>
                    <td className="p-3">
                      <span
                        className={cn(
                          "text-xs font-medium px-2 py-1 rounded",
                          project.type === "linked"
                            ? "bg-blue-500/10 text-blue-400"
                            : "bg-indigo-500/10 text-indigo-400"
                        )}
                      >
                        {project.type}
                      </span>
                    </td>
                    <td className="p-3">
                      {project.secure ? (
                        <Lock size={16} className="text-green-400" />
                      ) : (
                        <LockOpen
                          size={16}
                          className="text-[var(--muted-foreground)]"
                        />
                      )}
                    </td>
                    <td className="p-3">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() =>
                            handleOpenUrl(project.domain, project.secure)
                          }
                          className="p-2 rounded-lg hover:bg-[var(--card-hover)] transition-colors text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                          title="Open in browser"
                        >
                          <ExternalLink size={16} />
                        </button>
                        <button
                          onClick={() => handleRemoveProject(project)}
                          className="p-2 rounded-lg hover:bg-red-500/10 transition-colors text-[var(--muted-foreground)] hover:text-red-400"
                          title="Remove"
                        >
                          <Trash2 size={16} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="text-center py-12 text-[var(--muted-foreground)]">
            <FolderOpen size={48} className="mx-auto mb-4 opacity-50" />
            <p className="text-lg font-medium">No projects found</p>
            <p className="text-sm">
              {search
                ? "Try a different search term"
                : "Park a folder or link a project to get started"}
            </p>
          </div>
        )}
      </Card>
    </div>
  );
}
