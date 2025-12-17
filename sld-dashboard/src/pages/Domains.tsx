import { useMemo } from "react";
import {
  Globe,
  Lock,
  LockOpen,
  ExternalLink,
  RefreshCw,
  ShieldCheck,
} from "lucide-react";
import { Card, CardHeader } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useAppStore } from "@/stores/useAppStore";
import { cn } from "@/lib/utils";

export default function Domains() {
  const { projects, state, enableSecure } = useAppStore();

  const domains = useMemo(() => {
    return projects.map((project) => ({
      name: project.domain,
      projectName: project.name,
      path: project.path,
      secure: project.secure,
      type: project.type,
    }));
  }, [projects]);

  const handleOpenDomain = (domain: string, secure: boolean) => {
    const protocol = secure ? "https" : "http";
    window.open(`${protocol}://${domain}`, "_blank");
  };

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Domains</h1>
          <p className="text-[var(--muted-foreground)]">
            Manage your .{state?.tld || "test"} domains and SSL certificates
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary">
            <RefreshCw size={16} />
            Refresh DNS
          </Button>
          <Button variant="primary" onClick={enableSecure}>
            <ShieldCheck size={16} />
            Enable HTTPS
          </Button>
        </div>
      </div>

      {/* SSL Status Card */}
      <Card>
        <CardHeader
          title="SSL/HTTPS Status"
          description="Global certificate configuration"
          icon={<Lock size={20} />}
          action={
            <span
              className={cn(
                "text-xs font-medium px-3 py-1.5 rounded-full",
                state?.secure
                  ? "bg-green-500/10 text-green-400"
                  : "bg-amber-500/10 text-amber-400"
              )}
            >
              {state?.secure ? "Enabled" : "Disabled"}
            </span>
          }
        />
        <div className="p-4 rounded-lg bg-[var(--muted)]/30 border border-[var(--border)]">
          <div className="flex items-center gap-3">
            {state?.secure ? (
              <>
                <div className="p-2 rounded-lg bg-green-500/10">
                  <Lock size={20} className="text-green-400" />
                </div>
                <div>
                  <p className="font-medium text-[var(--foreground)]">
                    HTTPS is enabled globally
                  </p>
                  <p className="text-sm text-[var(--muted-foreground)]">
                    All .{state.tld} domains are secured with mkcert
                    certificates
                  </p>
                </div>
              </>
            ) : (
              <>
                <div className="p-2 rounded-lg bg-amber-500/10">
                  <LockOpen size={20} className="text-amber-400" />
                </div>
                <div>
                  <p className="font-medium text-[var(--foreground)]">
                    HTTPS is not enabled
                  </p>
                  <p className="text-sm text-[var(--muted-foreground)]">
                    Click "Enable HTTPS" to install SSL certificates for all
                    domains
                  </p>
                </div>
              </>
            )}
          </div>
        </div>
      </Card>

      {/* Domains List */}
      <Card hover={false}>
        <CardHeader
          title="Active Domains"
          description={`${domains.length} domains configured`}
          icon={<Globe size={20} />}
        />

        {domains.length > 0 ? (
          <div className="space-y-3">
            {domains.map((domain) => (
              <div
                key={domain.name}
                className={cn(
                  "flex items-center justify-between p-4 rounded-lg",
                  "bg-[var(--muted)]/30 border border-[var(--border)]",
                  "transition-all duration-200 hover:bg-[var(--muted)]/50"
                )}
              >
                <div className="flex items-center gap-4">
                  <div
                    className={cn(
                      "p-2 rounded-lg",
                      domain.secure
                        ? "bg-green-500/10"
                        : "bg-[var(--secondary)]"
                    )}
                  >
                    {domain.secure ? (
                      <Lock size={18} className="text-green-400" />
                    ) : (
                      <Globe
                        size={18}
                        className="text-[var(--muted-foreground)]"
                      />
                    )}
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <code className="font-mono font-medium text-[var(--primary)]">
                        {domain.name}
                      </code>
                      <span
                        className={cn(
                          "text-xs px-2 py-0.5 rounded",
                          domain.type === "linked"
                            ? "bg-blue-500/10 text-blue-400"
                            : "bg-indigo-500/10 text-indigo-400"
                        )}
                      >
                        {domain.type}
                      </span>
                    </div>
                    <p className="text-xs text-[var(--muted-foreground)] font-mono">
                      {domain.path}
                    </p>
                  </div>
                </div>

                <button
                  onClick={() => handleOpenDomain(domain.name, domain.secure)}
                  className={cn(
                    "flex items-center gap-2 px-3 py-1.5 rounded-lg",
                    "text-sm font-medium",
                    "bg-[var(--secondary)] text-[var(--secondary-foreground)]",
                    "hover:bg-[var(--card-hover)] transition-colors"
                  )}
                >
                  <ExternalLink size={14} />
                  Open
                </button>
              </div>
            ))}
          </div>
        ) : (
          <div className="text-center py-12 text-[var(--muted-foreground)]">
            <Globe size={48} className="mx-auto mb-4 opacity-50" />
            <p className="text-lg font-medium">No domains configured</p>
            <p className="text-sm">
              Park a folder or link a project to create domains
            </p>
          </div>
        )}
      </Card>
    </div>
  );
}
