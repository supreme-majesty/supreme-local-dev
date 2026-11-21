import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Camera, Database, GitBranch, History, Trash2, Undo2 } from "lucide-react";
import {
  ApiError,
  v2,
  type DBCheckpoint,
  type DBInfo,
  type DBServer,
  type JobView,
  type ProjectView,
} from "@/api/v2";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { JobConsole } from "@/components/projects/JobConsole";
import { useToast } from "@/hooks/useToast";
import { formatBytes } from "@/lib/format";
import { cn } from "@/lib/utils";

interface Props {
  project: ProjectView;
  /** Restarts the project (offered after switching its database). */
  onRestart?: () => void;
}

type Dialog =
  | { kind: "checkpoint"; server: DBServer }
  | { kind: "branch"; server: DBServer; checkpoint?: DBCheckpoint }
  | { kind: "restore"; checkpoint: DBCheckpoint }
  | { kind: "drop"; server: DBServer; db: DBInfo }
  | { kind: "use"; server: DBServer; db: DBInfo }
  | { kind: "deleteCheckpoint"; checkpoint: DBCheckpoint };

const engineLabel: Record<DBServer["engine"], string> = { mysql: "MySQL", mariadb: "MariaDB", postgres: "PostgreSQL" };

/**
 * Checkpoints and branches of the project's own database: snapshot it before
 * something risky, roll back, or try changes on a copy and point the app at it.
 */
export function DatabaseBranches({ project, onRestart }: Props) {
  const queryClient = useQueryClient();
  const { success, error: toastError } = useToast();
  const [job, setJob] = useState<{ id: string; title: string } | null>(null);
  const [dialog, setDialog] = useState<Dialog | null>(null);
  const [busy, setBusy] = useState(false);

  const { data, isLoading, error } = useQuery({
    queryKey: ["v2", "db", project.id],
    queryFn: () => v2.db(project.id),
    refetchInterval: 15000,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["v2", "db", project.id] });

  const startJob = async (title: string, started: Promise<JobView>) => {
    setBusy(true);
    try {
      const j = await started;
      setJob({ id: j.id, title });
      setDialog(null);
    } catch (e) {
      if (e instanceof ApiError && e.status === 409 && e.jobId) {
        setJob({ id: e.jobId, title: "Database operation in progress" });
        setDialog(null);
      } else {
        toastError("Could not start", (e as Error).message);
      }
    } finally {
      setBusy(false);
    }
  };

  const act = async (fn: () => Promise<unknown>, done: string, after?: () => void) => {
    setBusy(true);
    try {
      await fn();
      success(done);
      setDialog(null);
      refresh();
      after?.();
    } catch (e) {
      toastError("Failed", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (isLoading) return <p className="text-sm text-[var(--muted-foreground)]">Looking for the project's database…</p>;
  if (error) return <p className="text-sm text-red-400">{(error as Error).message}</p>;

  const servers = data?.servers ?? [];
  const checkpoints = data?.checkpoints ?? [];

  if (servers.length === 0) {
    return (
      <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-6 text-sm text-[var(--muted-foreground)]">
        <Database size={20} className="mb-2" />
        This project has no database container (MySQL, MariaDB or PostgreSQL) of its own.{" "}
        {project.containers === 0
          ? "Start the project to create its containers."
          : "Add one with services.database in .sld/config.yaml, or a database service in its compose file."}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-[var(--muted-foreground)]">
        Take a checkpoint before a risky migration and roll back in one step, or try changes on a branch (a copy named{" "}
        <code>database__branch</code>) and point the app at it. Everything runs inside the database container; checkpoints
        are kept in SLD's data volume.
      </p>

      {job && (
        <JobConsole
          key={job.id}
          jobId={job.id}
          title={job.title}
          onClose={() => setJob(null)}
          onFinished={(done) => {
            refresh();
            if (done.status === "succeeded") success("Done", job.title);
            else toastError("Database operation failed", done.error);
          }}
        />
      )}

      {servers.map((srv) => (
        <ServerCard
          key={srv.container}
          server={srv}
          envFile={data?.env_file}
          checkpoints={checkpoints.filter((c) => c.container === srv.container)}
          onDialog={setDialog}
        />
      ))}

      {/* Dialogs */}
      {dialog?.kind === "checkpoint" && (
        <CheckpointDialog
          server={dialog.server}
          busy={busy}
          onClose={() => setDialog(null)}
          onSubmit={(database, label) =>
            startJob(`Checkpoint of ${database}`, v2.dbCheckpoint(project.id, { container: dialog.server.container, database, label }))
          }
        />
      )}
      {dialog?.kind === "branch" && (
        <BranchDialog
          server={dialog.server}
          checkpoint={dialog.checkpoint}
          busy={busy}
          onClose={() => setDialog(null)}
          onSubmit={(name) =>
            startJob(
              `Branch ${name}`,
              v2.dbBranch(project.id, {
                container: dialog.server.container,
                database: dialog.server.main,
                name,
                checkpoint: dialog.checkpoint?.id,
              }),
            )
          }
        />
      )}
      {dialog?.kind === "restore" && (
        <Confirm
          title="Restore checkpoint"
          confirm="Restore"
          danger
          busy={busy}
          onClose={() => setDialog(null)}
          onConfirm={() =>
            startJob(`Restore ${dialog.checkpoint.database}`, v2.dbRestore(project.id, dialog.checkpoint.id))
          }
        >
          <p>
            <strong>{dialog.checkpoint.database}</strong> is replaced by its state at{" "}
            {new Date(dialog.checkpoint.created_at).toLocaleString()}
            {dialog.checkpoint.label ? ` (“${dialog.checkpoint.label}”)` : ""}. Changes made since are removed from it.
          </p>
          <p className="mt-2">
            SLD takes a checkpoint of the current state first, so you can undo this by restoring that one.
          </p>
        </Confirm>
      )}
      {dialog?.kind === "drop" && (
        <Confirm
          title="Delete branch"
          confirm="Delete branch"
          danger
          busy={busy}
          onClose={() => setDialog(null)}
          onConfirm={() =>
            act(() => v2.dbDrop(project.id, dialog.server.container, dialog.db.name), `Deleted ${dialog.db.name}`)
          }
        >
          <p>
            The database <strong>{dialog.db.name}</strong> and everything in it is deleted. This cannot be undone (take a
            checkpoint of it first if you may need it).
          </p>
        </Confirm>
      )}
      {dialog?.kind === "use" && (
        <Confirm
          title={dialog.db.is_main ? "Switch back to the main database" : "Use this branch"}
          confirm="Switch"
          busy={busy}
          onClose={() => setDialog(null)}
          onConfirm={() =>
            act(
              () => v2.dbUse(project.id, dialog.server.container, dialog.db.name),
              `${data?.env_file ?? ".env"} now uses ${dialog.db.name}`,
            )
          }
        >
          <p>
            <code>DB_DATABASE</code> in <code>{data?.env_file ?? ".env"}</code> is set to <strong>{dialog.db.name}</strong>
            . The previous file is backed up (Environment tab → backups).
          </p>
          <p className="mt-2">
            Restart the project afterwards if the app reads its settings only at startup (or has cached its config).
          </p>
          {onRestart && (
            <p className="mt-2">
              <button
                type="button"
                className="text-blue-400 hover:underline"
                onClick={() =>
                  act(
                    () => v2.dbUse(project.id, dialog.server.container, dialog.db.name),
                    `${data?.env_file ?? ".env"} now uses ${dialog.db.name}`,
                    onRestart,
                  )
                }
              >
                Switch and restart the project
              </button>
            </p>
          )}
        </Confirm>
      )}
      {dialog?.kind === "deleteCheckpoint" && (
        <Confirm
          title="Delete checkpoint"
          confirm="Delete"
          danger
          busy={busy}
          onClose={() => setDialog(null)}
          onConfirm={() =>
            act(() => v2.dbDeleteCheckpoint(project.id, dialog.checkpoint.id), "Checkpoint deleted")
          }
        >
          <p>
            The checkpoint of <strong>{dialog.checkpoint.database}</strong> from{" "}
            {new Date(dialog.checkpoint.created_at).toLocaleString()} is deleted. This cannot be undone.
          </p>
        </Confirm>
      )}
    </div>
  );
}

function ServerCard({
  server: srv,
  envFile,
  checkpoints,
  onDialog,
}: {
  server: DBServer;
  envFile?: string;
  checkpoints: DBCheckpoint[];
  onDialog: (d: Dialog) => void;
}) {
  const running = srv.state === "running";
  const main = srv.databases.find((d) => d.is_main);
  const branches = srv.databases.filter((d) => d.branch);
  const inUse = srv.databases.find((d) => d.in_use);

  return (
    <section className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-4">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 text-sm font-semibold text-[var(--foreground)]">
            <Database size={15} /> {main?.name ?? srv.main ?? "Database"}
            {main && <span className="font-normal text-[var(--muted-foreground)]">{formatBytes(main.size)}</span>}
          </h2>
          <p className="mt-0.5 font-mono text-xs text-[var(--muted-foreground)]">
            {srv.container} · {engineLabel[srv.engine]}
          </p>
          {inUse && envFile && (
            <p className="mt-1 text-xs text-[var(--muted-foreground)]">
              {envFile} uses{" "}
              <span className={cn("font-medium", inUse.is_main ? "text-[var(--foreground)]" : "text-violet-400")}>
                {inUse.name}
              </span>
            </p>
          )}
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Button size="sm" variant="secondary" disabled={!running || !main} onClick={() => onDialog({ kind: "checkpoint", server: srv })}>
            <Camera size={14} /> Checkpoint
          </Button>
          <Button size="sm" disabled={!running || !main} onClick={() => onDialog({ kind: "branch", server: srv })}>
            <GitBranch size={14} /> New branch
          </Button>
        </div>
      </div>

      {!running && (
        <p className="mb-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-300">
          {srv.container} is {srv.state}. Start the project to take checkpoints, restore or branch.
        </p>
      )}
      {srv.error && <p className="mb-3 text-sm text-red-400">{srv.error}</p>}
      {running && !main && !srv.error && (
        <p className="mb-3 text-sm text-[var(--muted-foreground)]">
          SLD could not tell which database is the project's (set DB_DATABASE in its .env).
        </p>
      )}

      {/* Branches */}
      <h3 className="mb-1 mt-2 text-xs font-semibold uppercase tracking-wide text-[var(--muted-foreground)]">Branches</h3>
      {branches.length === 0 ? (
        <p className="mb-3 text-sm text-[var(--muted-foreground)]">No branches yet.</p>
      ) : (
        <ul className="mb-3 divide-y divide-[var(--border)]">
          {branches.map((b) => (
            <li key={b.name} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
              <span className="flex min-w-0 items-center gap-2">
                <GitBranch size={13} className="text-violet-400" />
                <span className="font-medium text-[var(--foreground)]">{b.branch}</span>
                <span className="font-mono text-xs text-[var(--muted-foreground)]">{b.name}</span>
                <span className="text-xs text-[var(--muted-foreground)]">{formatBytes(b.size)}</span>
                {b.in_use && (
                  <span className="rounded-full border border-violet-500/30 bg-violet-500/10 px-1.5 text-[10px] text-violet-300">
                    in use
                  </span>
                )}
              </span>
              <span className="flex gap-1.5">
                {envFile && running &&
                  (b.in_use ? (
                    main && (
                      <Button size="sm" variant="secondary" onClick={() => onDialog({ kind: "use", server: srv, db: main })}>
                        <Undo2 size={14} /> Back to {main.name}
                      </Button>
                    )
                  ) : (
                    <Button size="sm" variant="secondary" onClick={() => onDialog({ kind: "use", server: srv, db: b })}>
                      Use
                    </Button>
                  ))}
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!running || b.in_use}
                  title={b.in_use ? "Switch the project back to the main database first" : "Delete branch"}
                  onClick={() => onDialog({ kind: "drop", server: srv, db: b })}
                >
                  <Trash2 size={14} />
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {/* Checkpoints */}
      <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--muted-foreground)]">Checkpoints</h3>
      {checkpoints.length === 0 ? (
        <p className="text-sm text-[var(--muted-foreground)]">No checkpoints yet.</p>
      ) : (
        <ul className="divide-y divide-[var(--border)]">
          {checkpoints.map((c) => (
            <li key={c.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
              <span className="flex min-w-0 flex-wrap items-center gap-2">
                <History size={13} className="text-[var(--muted-foreground)]" />
                <span className="text-[var(--foreground)]">{new Date(c.created_at).toLocaleString()}</span>
                {c.label && <span className="truncate text-[var(--muted-foreground)]">“{c.label}”</span>}
                {c.auto && (
                  <span className="rounded-full border border-[var(--border)] px-1.5 text-[10px] text-[var(--muted-foreground)]" title="Taken automatically before a restore">
                    auto
                  </span>
                )}
                <span className="font-mono text-xs text-[var(--muted-foreground)]">{c.database}</span>
                <span className="text-xs text-[var(--muted-foreground)]">{formatBytes(c.size)}</span>
              </span>
              <span className="flex gap-1.5">
                <Button size="sm" variant="secondary" disabled={!running} onClick={() => onDialog({ kind: "restore", checkpoint: c })}>
                  <Undo2 size={14} /> Restore
                </Button>
                <Button size="sm" variant="secondary" disabled={!running} onClick={() => onDialog({ kind: "branch", server: srv, checkpoint: c })}>
                  <GitBranch size={14} /> Branch
                </Button>
                <Button size="sm" variant="ghost" title="Delete checkpoint" onClick={() => onDialog({ kind: "deleteCheckpoint", checkpoint: c })}>
                  <Trash2 size={14} />
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function CheckpointDialog({
  server,
  busy,
  onClose,
  onSubmit,
}: {
  server: DBServer;
  busy: boolean;
  onClose: () => void;
  onSubmit: (database: string, label: string) => void;
}) {
  const candidates = server.databases.filter((d) => d.is_main || d.branch);
  const [database, setDatabase] = useState(server.main ?? candidates[0]?.name ?? "");
  const [label, setLabel] = useState("");
  return (
    <Modal
      isOpen
      onClose={onClose}
      title="Take a checkpoint"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={busy || !database} onClick={() => onSubmit(database, label.trim())}>
            <Camera size={14} /> Take checkpoint
          </Button>
        </div>
      }
    >
      <div className="space-y-3 text-sm">
        {candidates.length > 1 && (
          <label className="block">
            <span className="mb-1 block text-[var(--muted-foreground)]">Database</span>
            <select
              value={database}
              onChange={(e) => setDatabase(e.target.value)}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 py-1.5 text-[var(--foreground)]"
            >
              {candidates.map((d) => (
                <option key={d.name} value={d.name}>
                  {d.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="block">
          <span className="mb-1 block text-[var(--muted-foreground)]">Label (optional)</span>
          <input
            autoFocus
            value={label}
            maxLength={100}
            onChange={(e) => setLabel(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && !busy && database && onSubmit(database, label.trim())}
            placeholder="e.g. before the orders migration"
            className="w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 py-1.5 text-[var(--foreground)]"
          />
        </label>
      </div>
    </Modal>
  );
}

const branchNameRe = /^[a-z0-9][a-z0-9_]{0,29}$/;

function BranchDialog({
  server,
  checkpoint,
  busy,
  onClose,
  onSubmit,
}: {
  server: DBServer;
  checkpoint?: DBCheckpoint;
  busy: boolean;
  onClose: () => void;
  onSubmit: (name: string) => void;
}) {
  const [name, setName] = useState("");
  const base = checkpoint?.database.split("__")[0] ?? server.main ?? "";
  const valid = branchNameRe.test(name);
  const exists = server.databases.some((d) => d.name === `${base}__${name}`);
  const submit = () => valid && !exists && !busy && onSubmit(name);
  return (
    <Modal
      isOpen
      onClose={onClose}
      title="New branch"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!valid || exists || busy} onClick={submit}>
            <GitBranch size={14} /> Create branch
          </Button>
        </div>
      }
    >
      <div className="space-y-3 text-sm">
        <p className="text-[var(--muted-foreground)]">
          {checkpoint ? (
            <>
              A copy of <strong className="text-[var(--foreground)]">{checkpoint.database}</strong> as it was at{" "}
              {new Date(checkpoint.created_at).toLocaleString()}.
            </>
          ) : (
            <>
              A copy of <strong className="text-[var(--foreground)]">{server.main}</strong> as it is now.
            </>
          )}{" "}
          The original is not changed.
        </p>
        <label className="block">
          <span className="mb-1 block text-[var(--muted-foreground)]">Branch name</span>
          <input
            autoFocus
            value={name}
            onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_"))}
            onKeyDown={(e) => e.key === "Enter" && submit()}
            placeholder="e.g. new_pricing"
            className="w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 py-1.5 font-mono text-[var(--foreground)]"
          />
        </label>
        <p className="text-xs text-[var(--muted-foreground)]">
          {name && !valid
            ? "Use lowercase letters, digits and _ (max 30), starting with a letter or digit."
            : exists
              ? "A branch with this name already exists."
              : `Database: ${base}__${name || "…"}`}
        </p>
      </div>
    </Modal>
  );
}

function Confirm({
  title,
  confirm,
  danger,
  busy,
  onClose,
  onConfirm,
  children,
}: {
  title: string;
  confirm: string;
  danger?: boolean;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
  children: React.ReactNode;
}) {
  return (
    <Modal
      isOpen
      onClose={onClose}
      title={title}
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant={danger ? "danger" : "primary"} disabled={busy} onClick={onConfirm}>
            {confirm}
          </Button>
        </div>
      }
    >
      <div className="text-sm text-[var(--muted-foreground)] [&_strong]:text-[var(--foreground)]">{children}</div>
    </Modal>
  );
}
