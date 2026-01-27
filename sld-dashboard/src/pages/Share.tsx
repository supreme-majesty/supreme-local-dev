import { useState, useEffect } from "react";
import {
  Globe,
  Link2,
  Copy,
  ExternalLink,
  Play,
  Square,
  Loader2,
  CheckCircle2,
  Wifi,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface Tunnel {
  site: string;
  url: string;
  status: "active" | "starting" | "stopped";
}

interface Site {
  name: string;
  domain: string;
  path: string;
  type: "parked" | "linked";
}

export default function Share() {
  const [sites, setSites] = useState<Site[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [loading, setLoading] = useState(true);
  const [starting, setStarting] = useState<string | null>(null);
  const [stopping, setStopping] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const fetchData = async () => {
    try {
      const [sitesRes, tunnelsRes] = await Promise.all([
        fetch("/api/sites"),
        fetch("/api/share/status"),
      ]);
      const sitesData = await sitesRes.json();
      const tunnelsData = await tunnelsRes.json();
      setSites(sitesData);
      setTunnels(tunnelsData || []);
    } catch (error) {
      console.error("Failed to fetch data:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, []);

  const startTunnel = async (siteName: string) => {
    setStarting(siteName);
    try {
      const res = await fetch("/api/share/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ site: siteName }),
      });
      const data = await res.json();
      if (data.success) {
        await fetchData();
      } else {
        alert(`Failed to start tunnel: ${data.error}`);
      }
    } catch (error) {
      console.error("Failed to start tunnel:", error);
    } finally {
      setStarting(null);
    }
  };

  const stopTunnel = async (siteName: string) => {
    setStopping(siteName);
    try {
      const res = await fetch("/api/share/stop", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ site: siteName }),
      });
      const data = await res.json();
      if (data.success) {
        await fetchData();
      } else {
        alert(`Failed to stop tunnel: ${data.error}`);
      }
    } catch (error) {
      console.error("Failed to stop tunnel:", error);
    } finally {
      setStopping(null);
    }
  };

  const copyToClipboard = async (text: string, site: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(site);
      setTimeout(() => setCopied(null), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  const getTunnelForSite = (siteName: string) => {
    return tunnels.find((t) => t.site === siteName);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center shadow-lg">
            <Globe size={28} className="text-white" />
          </div>
          <div>
            <h1 className="text-2xl font-bold">Share Projects</h1>
            <p className="text-sm text-[var(--muted-foreground)]">
              Share your local sites via secure public URLs using Cloudflare
              Tunnel
            </p>
          </div>
        </div>
      </div>

      {/* Active Tunnels Summary */}
      {tunnels.length > 0 && (
        <div className="bg-gradient-to-r from-emerald-500/10 to-teal-500/10 border border-emerald-500/20 rounded-xl p-4">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-emerald-500/20 flex items-center justify-center">
              <Wifi size={20} className="text-emerald-500" />
            </div>
            <div>
              <h3 className="font-semibold text-emerald-500">
                {tunnels.length} Active Tunnel{tunnels.length > 1 ? "s" : ""}
              </h3>
              <p className="text-sm text-[var(--muted-foreground)]">
                Your sites are being shared publicly
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Sites Grid */}
      <div className="grid gap-4">
        {sites.length === 0 ? (
          <div className="bg-[var(--card)] rounded-xl border border-[var(--border)] p-12 text-center">
            <Globe
              size={48}
              className="mx-auto text-[var(--muted-foreground)] opacity-50 mb-4"
            />
            <h3 className="text-lg font-semibold mb-2">No Sites Available</h3>
            <p className="text-sm text-[var(--muted-foreground)]">
              Park or link a project first to share it publicly
            </p>
          </div>
        ) : (
          sites.map((site) => {
            const tunnel = getTunnelForSite(site.name);
            const isActive = !!tunnel;
            const isStarting = starting === site.name;
            const isStopping = stopping === site.name;

            return (
              <div
                key={site.name}
                className={cn(
                  "bg-[var(--card)] rounded-xl border p-5 transition-all duration-300",
                  isActive
                    ? "border-emerald-500/50 shadow-lg shadow-emerald-500/5"
                    : "border-[var(--border)] hover:border-[var(--border-hover)]"
                )}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div
                      className={cn(
                        "w-12 h-12 rounded-xl flex items-center justify-center transition-colors",
                        isActive
                          ? "bg-emerald-500/10 text-emerald-500"
                          : "bg-[var(--card-hover)] text-[var(--muted-foreground)]"
                      )}
                    >
                      <Link2 size={24} />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <h3 className="font-semibold">{site.name}</h3>
                        {isActive && (
                          <span className="flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold uppercase bg-emerald-500/10 text-emerald-500">
                            <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                            Live
                          </span>
                        )}
                      </div>
                      <p className="text-sm text-[var(--muted-foreground)]">
                        {site.domain}
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    {isActive && tunnel?.url && (
                      <div className="flex items-center gap-2 bg-[var(--background)] rounded-lg px-3 py-2 border border-[var(--border)]">
                        <span className="text-sm font-mono text-emerald-500 max-w-[200px] truncate">
                          {tunnel.url}
                        </span>
                        <button
                          onClick={() => copyToClipboard(tunnel.url, site.name)}
                          className="p-1.5 rounded-md hover:bg-[var(--card-hover)] transition-colors"
                          title="Copy URL"
                        >
                          {copied === site.name ? (
                            <CheckCircle2
                              size={16}
                              className="text-emerald-500"
                            />
                          ) : (
                            <Copy
                              size={16}
                              className="text-[var(--muted-foreground)]"
                            />
                          )}
                        </button>
                        <a
                          href={tunnel.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="p-1.5 rounded-md hover:bg-[var(--card-hover)] transition-colors"
                          title="Open in new tab"
                        >
                          <ExternalLink
                            size={16}
                            className="text-[var(--muted-foreground)]"
                          />
                        </a>
                      </div>
                    )}

                    {isActive ? (
                      <button
                        onClick={() => stopTunnel(site.name)}
                        disabled={isStopping}
                        className={cn(
                          "flex items-center gap-2 px-4 py-2 rounded-lg font-medium text-sm transition-all",
                          "bg-red-500/10 text-red-500 hover:bg-red-500/20",
                          isStopping && "opacity-50 cursor-not-allowed"
                        )}
                      >
                        {isStopping ? (
                          <Loader2 size={16} className="animate-spin" />
                        ) : (
                          <Square size={16} />
                        )}
                        {isStopping ? "Stopping..." : "Stop"}
                      </button>
                    ) : (
                      <button
                        onClick={() => startTunnel(site.name)}
                        disabled={isStarting}
                        className={cn(
                          "flex items-center gap-2 px-4 py-2 rounded-lg font-medium text-sm transition-all",
                          "bg-blue-500/10 text-blue-500 hover:bg-blue-500/20",
                          isStarting && "opacity-50 cursor-not-allowed"
                        )}
                      >
                        {isStarting ? (
                          <Loader2 size={16} className="animate-spin" />
                        ) : (
                          <Play size={16} />
                        )}
                        {isStarting ? "Starting..." : "Share"}
                      </button>
                    )}
                  </div>
                </div>

                {/* Path */}
                <div className="mt-3 pt-3 border-t border-[var(--border)]">
                  <p className="text-xs text-[var(--muted-foreground)] font-mono truncate">
                    {site.path}
                  </p>
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* Info */}
      <div className="bg-blue-500/5 border border-blue-500/20 rounded-xl p-4">
        <h4 className="font-semibold text-blue-500 mb-2">How it works</h4>
        <ul className="text-sm text-[var(--muted-foreground)] space-y-1">
          <li>
            • Tunnels are powered by Cloudflare's free Quick Tunnel feature
          </li>
          <li>
            • URLs are temporary and change each time you start a new tunnel
          </li>
          <li>
            • Perfect for sharing work with clients or testing on other devices
          </li>
          <li>
            • Tunnels remain active until you stop them or restart the daemon
          </li>
        </ul>
      </div>
    </div>
  );
}
