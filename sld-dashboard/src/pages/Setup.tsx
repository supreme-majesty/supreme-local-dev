import { useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Download, LogOut, Router, ShieldCheck, XCircle, AlertTriangle } from "lucide-react";
import { v2 } from "@/api/v2";
import { Button } from "@/components/ui/Button";
import { JobConsole } from "@/components/projects/JobConsole";
import { AutomationSettings } from "@/components/projects/AutomationSettings";

function Row({ ok, warn, label, children }: { ok: boolean; warn?: boolean; label: string; children: ReactNode }) {
  const Icon = ok ? CheckCircle2 : warn ? AlertTriangle : XCircle;
  const color = ok ? "text-green-400" : warn ? "text-amber-400" : "text-red-400";
  return (
    <div className="flex items-start gap-3 py-3 border-b border-[var(--border)] last:border-0">
      <Icon size={18} className={`${color} mt-0.5 flex-shrink-0`} />
      <div className="min-w-0">
        <div className="text-sm font-medium text-[var(--foreground)]">{label}</div>
        <div className="text-sm text-[var(--muted-foreground)]">{children}</div>
      </div>
    </div>
  );
}

const code = "px-1 py-0.5 rounded bg-[var(--secondary)] text-[var(--foreground)] text-xs";

export default function Setup() {
  const queryClient = useQueryClient();
  const [jobId, setJobId] = useState<string | null>(null);
  const { data: sys, error, isLoading } = useQuery({ queryKey: ["v2", "system"], queryFn: v2.system, refetchInterval: 10000 });

  const initRouter = async () => {
    const job = await v2.initRouter();
    setJobId(job.id);
  };

  if (isLoading) return <p className="text-sm text-[var(--muted-foreground)]">Loading…</p>;
  if (error || !sys) return <p className="text-sm text-red-400">Could not load system status: {(error as Error)?.message}</p>;

  const routerOk = sys.router === "running";
  const portSuffix = sys.http_port === 80 ? "" : `:${sys.http_port}`;
  const workspaces = sys.workspaces ?? [];

  return (
    <div className="space-y-6 max-w-3xl">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-[var(--foreground)]">Setup</h1>
          <p className="text-sm text-[var(--muted-foreground)]">
            Check that SLD can reach Docker, serve your domains and see your projects.
          </p>
        </div>
        <Button variant="ghost" onClick={() => v2.logout().finally(() => window.location.reload())}>
          <LogOut size={16} /> Sign out
        </Button>
      </div>

      <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] px-4">
        <Row ok={sys.docker} label="Docker">
          {sys.docker ? "Connected to the Docker engine." : (
            <>SLD cannot reach Docker. Start the SLD container with <code className={code}>-v /var/run/docker.sock:/var/run/docker.sock</code>.</>
          )}
        </Row>
        <Row ok={routerOk} label="Router">
          {routerOk ? (
            <>
              Serving <code className={code}>*.{sys.tld}{portSuffix}</code> on {sys.bind_host}:{sys.http_port} (HTTP) and{" "}
              {sys.https_port} (HTTPS).
              {sys.http_port !== 80 && " Port 80 was busy when SLD was set up, so URLs include the port."}
            </>
          ) : (
            <span className="flex flex-col gap-2">
              <span>The router is {sys.router}. It routes every project domain; start it before starting projects.</span>
              <span>
                <Button size="sm" onClick={initRouter}>
                  <Router size={14} /> Start router
                </Button>
              </span>
            </span>
          )}
        </Row>
        <Row ok={workspaces.length > 0} label="Workspace">
          {workspaces.length > 0 ? (
            <>
              Projects are discovered in {workspaces.map((w) => <code key={w} className={`${code} mr-1`}>{w}</code>)}
              {sys.mounts?.filter((m) => workspaces.some((w) => w === m.destination)).map((m) => (
                <span key={m.destination}> (your folder {m.source})</span>
              ))}
            </>
          ) : (
            <>No workspace folder is mounted. Add <code className={code}>-v &lt;your projects folder&gt;:/workspace</code> when starting SLD.</>
          )}
        </Row>
        <Row ok={Boolean(sys.ca_available)} warn={!sys.ca_available} label="HTTPS certificates">
          {sys.ca_available ? (
            <span className="flex flex-col gap-2">
              <span>
                Trust SLD&apos;s certificate authority once so browsers accept <code className={code}>https://</code> project
                URLs. Plain <code className={code}>http://</code> works without it.
              </span>
              <span>
                <a href={v2.caCertUrl} download="sld-ca.crt">
                  <Button size="sm" variant="secondary">
                    <Download size={14} /> Download CA certificate
                  </Button>
                </a>
              </span>
              <span className="text-xs">
                Windows: open the file → Install Certificate → Local Machine → “Trusted Root Certification Authorities”.
                macOS: open it in Keychain Access and set it to “Always Trust”. Firefox: Settings → Certificates → Import.
              </span>
            </span>
          ) : (
            "The certificate authority is created when the router is first started."
          )}
        </Row>
      </div>

      <AutomationSettings />

      {jobId && (
        <JobConsole
          jobId={jobId}
          title="Starting router"
          onClose={() => setJobId(null)}
          onFinished={() => queryClient.invalidateQueries({ queryKey: ["v2", "system"] })}
        />
      )}

      <div className="flex items-center gap-2 text-xs text-[var(--muted-foreground)]">
        <ShieldCheck size={14} /> SLD {sys.version} · data in <code className={code}>{sys.data_dir}</code> ·{" "}
        {sys.in_container ? "running in Docker" : "running on the host"}
      </div>
    </div>
  );
}
