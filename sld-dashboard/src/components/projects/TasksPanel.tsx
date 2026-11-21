import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Play, Terminal } from "lucide-react";
import { v2, type JobView, type ProjectView, type TaskView } from "@/api/v2";
import { Button } from "@/components/ui/Button";
import { JobConsole } from "@/components/projects/JobConsole";
import { useToast } from "@/hooks/useToast";

interface Props {
  project: ProjectView;
}

/**
 * One-off commands inside the project's app containers: the tasks from the
 * project's `tasks:` block, ones SLD suggests (migrations, package.json
 * scripts...), and a free-form command.
 */
export function TasksPanel({ project }: Props) {
  const { error: toastError } = useToast();
  const isUp = project.status === "running" || project.status === "partial";
  const [job, setJob] = useState<{ id: string; title: string } | null>(null);
  const [command, setCommand] = useState("");
  const [app, setApp] = useState("");

  const { data, isLoading } = useQuery({
    queryKey: ["v2", "tasks", project.id],
    queryFn: () => v2.tasks(project.id),
    enabled: project.configured,
  });
  const tasks = data?.tasks ?? [];
  const apps = data?.apps ?? [];

  const start = async (title: string, started: Promise<JobView>) => {
    try {
      const j = await started;
      setJob({ id: j.id, title });
    } catch (e) {
      toastError("Could not run the command", (e as Error).message);
    }
  };

  const runTask = (t: TaskView) => start(`${t.app ? `${t.app}: ` : ""}${t.command}`, v2.runTask(project.id, t.name, t.app));

  const runCommand = (e: React.FormEvent) => {
    e.preventDefault();
    const cmd = command.trim();
    if (!cmd) return;
    start(`${app ? `${app}: ` : ""}${cmd}`, v2.runCommand(project.id, cmd, app || undefined));
  };

  if (!project.configured) {
    return <p className="text-sm text-[var(--muted-foreground)]">Set the project up to run tasks in its containers.</p>;
  }

  const byApp = new Map<string, TaskView[]>();
  for (const t of tasks) {
    const key = t.app || apps[0] || "";
    byApp.set(key, [...(byApp.get(key) ?? []), t]);
  }

  return (
    <div className="space-y-4">
      {!isUp && (
        <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-300">
          Start the project to run tasks: they run inside its running containers.
        </p>
      )}

      {job && <JobConsole key={job.id} jobId={job.id} title={job.title} onClose={() => setJob(null)} />}

      <section className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-4">
        <h2 className="mb-3 flex items-center gap-1.5 text-sm font-semibold text-[var(--foreground)]">
          <Terminal size={14} /> Run a command
        </h2>
        <form onSubmit={runCommand} className="flex flex-wrap gap-2">
          {apps.length > 1 && (
            <select
              value={app}
              onChange={(e) => setApp(e.target.value)}
              aria-label="App to run the command in"
              className="rounded-md border border-[var(--border)] bg-[var(--background)] px-2 py-1.5 text-sm text-[var(--foreground)]"
            >
              <option value="">{apps[0]} (main)</option>
              {apps.slice(1).map((a) => (
                <option key={a} value={a}>
                  {a}
                </option>
              ))}
            </select>
          )}
          <input
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder="e.g. php artisan tinker --execute='echo 1;'  ·  npm install axios"
            aria-label="Command"
            className="min-w-0 flex-1 rounded-md border border-[var(--border)] bg-[var(--background)] px-3 py-1.5 font-mono text-sm text-[var(--foreground)]"
          />
          <Button size="sm" type="submit" disabled={!isUp || !command.trim()}>
            <Play size={14} /> Run
          </Button>
        </form>
        <p className="mt-2 text-xs text-[var(--muted-foreground)]">
          Runs with <code>sh -c</code> in the app's container, as the project folder's owner for SLD runtimes. Interactive
          commands are not supported; use a terminal for those.
        </p>
      </section>

      <section className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-4">
        <h2 className="mb-1 text-sm font-semibold text-[var(--foreground)]">Tasks</h2>
        <p className="mb-3 text-xs text-[var(--muted-foreground)]">
          Add your own under <code>tasks:</code> in <code>.sld/config.yaml</code>, e.g.{" "}
          <code>fresh: php artisan migrate:fresh --seed</code>.
        </p>
        {isLoading ? (
          <p className="text-sm text-[var(--muted-foreground)]">Loading tasks…</p>
        ) : tasks.length === 0 ? (
          <p className="text-sm text-[var(--muted-foreground)]">{data?.error ?? "No tasks found for this project."}</p>
        ) : (
          <div className="space-y-4">
            {[...byApp.entries()].map(([appName, list]) => (
              <div key={appName}>
                {byApp.size > 1 && (
                  <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--muted-foreground)]">{appName}</h3>
                )}
                <ul className="divide-y divide-[var(--border)]">
                  {list.map((t) => (
                    <li key={`${t.app}/${t.name}`} className="flex items-center justify-between gap-3 py-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 text-sm text-[var(--foreground)]">
                          <span className="font-medium">{t.name}</span>
                          {t.source === "config" && (
                            <span className="rounded-full border border-blue-500/30 bg-blue-500/10 px-1.5 text-[10px] text-blue-300">
                              config
                            </span>
                          )}
                        </div>
                        <div className="truncate font-mono text-xs text-[var(--muted-foreground)]">{t.command}</div>
                        {t.description && <div className="text-xs text-[var(--muted-foreground)]">{t.description}</div>}
                      </div>
                      <Button size="sm" variant="secondary" disabled={!isUp} onClick={() => runTask(t)}>
                        <Play size={14} /> Run
                      </Button>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
