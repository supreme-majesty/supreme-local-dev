import { useMemo } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ExternalLink, Globe, Lock, RefreshCw, ShieldCheck } from "lucide-react";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { v2 } from "@/api/v2";
import { CaTrustNotice, HttpsSwitch } from "@/components/projects/HttpsCard";
import { useSetHttps } from "@/hooks/use-https";
import { cn } from "@/lib/utils";

export default function Domains() {
  const navigate = useNavigate();
  const { data: projects = [], refetch, isFetching } = useQuery({
    queryKey: ["v2", "projects"],
    queryFn: () => v2.projects(),
    refetchInterval: 15000,
  });
  const system = useQuery({ queryKey: ["v2", "system"], queryFn: () => v2.system() });
  const { setHttps, pending } = useSetHttps();

  const withDomains = useMemo(() => projects.filter((p) => (p.apps ?? []).length > 0), [projects]);
  const secureCount = withDomains.filter((p) => p.tls).length;

  return (
    <div className="space-y-6 animate-fade-in">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Domains</h1>
          <p className="text-[var(--muted-foreground)]">
            Your .{system.data?.tld || "localhost"} domains, and HTTPS for each project
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={() => refetch()} disabled={isFetching}>
            <RefreshCw size={16} />
            Refresh
          </Button>
          <Button variant="secondary" onClick={() => navigate("/setup")}>
            <ShieldCheck size={16} />
            Certificate setup
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-[var(--primary)]/10 text-[var(--primary)]">
              <Lock size={20} />
            </div>
            <div>
              <CardTitle className="text-lg">HTTPS</CardTitle>
              <CardDescription>Turned on per project with the switches below</CardDescription>
            </div>
          </div>
          <span
            className={cn(
              "text-xs font-medium px-3 py-1.5 rounded-full",
              secureCount > 0 ? "bg-green-500/10 text-green-400" : "bg-slate-500/10 text-slate-300",
            )}
          >
            {secureCount} of {withDomains.length} project{withDomains.length === 1 ? "" : "s"}
          </span>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-[var(--muted-foreground)]">
            Projects with HTTPS on are served at <code className="font-mono">https://&lt;name&gt;.{system.data?.tld || "localhost"}</code>
            {system.data?.https_port && system.data.https_port !== 443 ? `:${system.data.https_port}` : ""} with a
            certificate from SLD&apos;s own certificate authority. Plain HTTP keeps working.
          </p>
          <CaTrustNotice />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-lg bg-[var(--primary)]/10 text-[var(--primary)]">
              <Globe size={20} />
            </div>
            <div>
              <CardTitle className="text-lg">Domains</CardTitle>
              <CardDescription>
                {withDomains.length} project{withDomains.length === 1 ? "" : "s"}
              </CardDescription>
            </div>
          </div>
        </CardHeader>

        <CardContent>
          {withDomains.length > 0 ? (
            <div className="space-y-3">
              {withDomains.map((p) => {
                const isUp = p.status === "running" || p.status === "partial";
                return (
                  <div
                    key={p.id || p.folder}
                    className="flex flex-wrap items-center justify-between gap-3 p-4 rounded-lg bg-[var(--muted)]/30 border border-[var(--border)]"
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        {p.id ? (
                          <Link to={`/projects/${encodeURIComponent(p.id)}`} className="font-medium text-[var(--foreground)] hover:underline">
                            {p.name}
                          </Link>
                        ) : (
                          <span className="font-medium text-[var(--foreground)]">{p.name}</span>
                        )}
                        <span
                          className={cn(
                            "text-[11px] px-2 py-0.5 rounded-full",
                            isUp ? "bg-green-500/10 text-green-400" : "bg-slate-500/10 text-slate-300",
                          )}
                        >
                          {isUp ? "running" : p.configured ? "stopped" : "not set up"}
                        </span>
                      </div>
                      <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1">
                        {(p.apps ?? []).map((a) => (
                          <a
                            key={a.name}
                            href={a.url}
                            target="_blank"
                            rel="noreferrer"
                            className={cn(
                              "inline-flex items-center gap-1 font-mono text-xs",
                              isUp ? "text-[var(--primary)] hover:underline" : "text-[var(--muted-foreground)] pointer-events-none",
                            )}
                          >
                            {p.tls && <Lock size={11} className="text-green-400" />}
                            {a.url.replace(/^https?:\/\//, "")}
                            {isUp && <ExternalLink size={11} />}
                          </a>
                        ))}
                      </div>
                    </div>
                    <div className="flex items-center gap-2 text-xs text-[var(--muted-foreground)]">
                      HTTPS
                      <HttpsSwitch project={p} pending={pending.has(p.id)} onChange={(on) => setHttps(p, on)} />
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="text-center py-12 text-[var(--muted-foreground)]">
              <Globe size={48} className="mx-auto mb-4 opacity-50" />
              <p className="text-lg font-medium">No domains yet</p>
              <p className="text-sm">Projects get a domain once they are detected on the Projects page</p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
