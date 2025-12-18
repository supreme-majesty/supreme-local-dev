import { useEffect, useState } from "react";
import { Card, CardHeader } from "@/components/ui/Card";
import { Activity, Cpu, Server, Network } from "lucide-react";

interface MetricsData {
  cpu_percent: number;
  ram_usage: string;
  ram_total: string;
  ram_percent: number;
  active_connections: number;
  sites_parked: number;
  sites_linked: number;
  services_running: number;
}

const Metrics = () => {
  const [data, setData] = useState<MetricsData | null>(null);

  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        const res = await fetch("/api/metrics");
        if (res.ok) {
          const json = await res.json();
          setData(json);
        }
      } catch (e) {
        console.error("Failed to fetch metrics", e);
      }
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 3000);
    return () => clearInterval(interval);
  }, []);

  if (!data) return null;

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 animate-fade-in">
      {/* CPU Usage */}
      <Card>
        <CardHeader title="CPU Usage" icon={<Cpu className="h-4 w-4" />} />
        <div>
          <div className="text-2xl font-bold">
            {data.cpu_percent.toFixed(1)}%
          </div>
          <div className="h-2 w-full bg-[var(--muted)] rounded-full mt-2 overflow-hidden">
            <div
              className="bg-indigo-500 h-full transition-all duration-500 ease-out"
              style={{ width: `${Math.min(data.cpu_percent, 100)}%` }}
            />
          </div>
        </div>
      </Card>

      {/* RAM Usage */}
      <Card>
        <CardHeader title="RAM Usage" icon={<Activity className="h-4 w-4" />} />
        <div>
          <div className="text-2xl font-bold">{data.ram_usage}</div>
          <p className="text-xs text-[var(--muted-foreground)]">
            of {data.ram_total}
          </p>
          <div className="h-2 w-full bg-[var(--muted)] rounded-full mt-2 overflow-hidden">
            <div
              className="bg-emerald-500 h-full transition-all duration-500 ease-out"
              style={{ width: `${Math.min(data.ram_percent, 100)}%` }}
            />
          </div>
        </div>
      </Card>

      {/* Active Traffic */}
      <Card>
        <CardHeader
          title="Active Traffic"
          icon={<Network className="h-4 w-4" />}
        />
        <div>
          <div className="text-2xl font-bold">{data.active_connections}</div>
          <p className="text-xs text-[var(--muted-foreground)]">
            active connections
          </p>
        </div>
      </Card>

      {/* Services status */}
      <Card>
        <CardHeader
          title="System Health"
          icon={<Server className="h-4 w-4" />}
        />
        <div>
          <div className="text-2xl font-bold text-emerald-500">Good</div>
          <p className="text-xs text-[var(--muted-foreground)]">
            {data.services_running}/3 services running
          </p>
        </div>
      </Card>
    </div>
  );
};

export default Metrics;
