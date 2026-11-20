import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Download, Lock, LockOpen, ShieldCheck } from "lucide-react";
import { v2, type ProjectView } from "@/api/v2";
import { Switch } from "@/components/ui/Switch";
import { useCaTrust } from "@/hooks/use-https";
import { cn } from "@/lib/utils";

/** The browser-trust state of SLD's certificate authority, with what to do about it. */
export function CaTrustNotice({ compact = false }: { compact?: boolean }) {
  const system = useQuery({ queryKey: ["v2", "system"], queryFn: () => v2.system() });
  const trust = useCaTrust(system.data?.https_port, system.data?.router === "running");

  if (trust.data === "trusted") {
    return (
      <p className="flex items-center gap-1.5 text-xs text-green-400">
        <ShieldCheck size={14} /> This browser trusts SLD&apos;s certificate authority.
      </p>
    );
  }
  if (system.data && system.data.router !== "running") {
    return (
      <p className="flex items-center gap-1.5 text-xs text-[var(--muted-foreground)]">
        <AlertTriangle size={14} /> The router is not running, so HTTPS cannot be checked.{" "}
        <Link to="/setup" className="text-blue-400 hover:underline">
          Open Setup
        </Link>
      </p>
    );
  }
  if (trust.data !== "untrusted") return null;
  return (
    <div className={cn("rounded-md border border-amber-500/30 bg-amber-500/10 text-amber-300", compact ? "p-2 text-xs" : "p-3 text-sm")}>
      <p className="flex items-start gap-1.5">
        <AlertTriangle size={14} className="mt-0.5 flex-shrink-0" />
        <span>
          This browser does not trust SLD&apos;s certificate authority yet, so HTTPS projects show a certificate warning.
          Install it once:{" "}
          <a href={v2.caCertUrl} download="sld-ca.crt" className="inline-flex items-center gap-1 underline">
            <Download size={12} /> download the certificate
          </a>
          , then follow the steps on{" "}
          <Link to="/setup" className="underline">
            Setup
          </Link>
          . This page notices when you come back.
        </span>
      </p>
    </div>
  );
}

/** HTTPS switch for one project. */
export function HttpsSwitch({
  project,
  pending,
  onChange,
}: {
  project: ProjectView;
  pending: boolean;
  onChange: (enabled: boolean) => void;
}) {
  const title = !project.configured
    ? "Set the project up first"
    : project.tls
      ? "HTTPS is on — click to turn it off"
      : "Turn HTTPS on";
  return (
    <span className="inline-flex items-center gap-2" title={title}>
      {project.tls ? <Lock size={14} className="text-green-400" /> : <LockOpen size={14} className="text-[var(--muted-foreground)]" />}
      <Switch checked={project.tls} onCheckedChange={onChange} disabled={!project.configured} loading={pending} />
    </span>
  );
}
