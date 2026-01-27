import { useEffect, useState } from "react";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Activity, Play, Square, RotateCw, Server } from "lucide-react";
import { api } from "@/api/daemon";
import type { ServiceStatus, HealthCheck } from "@/api/daemon";

export default function SystemStatus() {
  const [services, setServices] = useState<ServiceStatus[]>([]);
  const [health, setHealth] = useState<HealthCheck[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchData = async () => {
    try {
      const [services, health] = await Promise.all([
        api.getServiceStatus(),
        api.runDoctor(),
      ]);
      setServices(services);
      setHealth(health);
    } catch (error) {
      console.error("Failed to fetch system status", error);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleAction = async (
    service: string,
    action: "start" | "stop" | "restart"
  ) => {
    setLoading(true);
    try {
      await api.controlService(service, action);
      await fetchData();
    } catch (error) {
      console.error(`Failed to ${action} ${service}`, error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-500">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-white mb-2">
            System Status
          </h1>
          <p className="text-zinc-400">
            Monitor and control core system services.
          </p>
        </div>
        <Button onClick={() => fetchData()} variant="outline" size="sm">
          <RotateCw className="w-4 h-4 mr-2" />
          Refresh
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Core Services */}
        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader>
            <CardTitle className="flex items-center text-white">
              <Server className="w-5 h-5 mr-2 text-indigo-400" />
              Core Services
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {services.map((svc) => (
              <div
                key={svc.name}
                className="flex items-center justify-between p-3 bg-zinc-950/50 rounded-lg border border-zinc-900"
              >
                <div className="flex items-center gap-3">
                  <div
                    className={`w-2.5 h-2.5 rounded-full ${
                      svc.running
                        ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]"
                        : "bg-red-500"
                    }`}
                  />
                  <div>
                    <div className="font-medium text-zinc-100 flex items-center gap-2">
                      {svc.name}
                      {svc.version && (
                        <Badge
                          variant="secondary"
                          className="bg-zinc-800 text-xs py-0 h-5"
                        >
                          {svc.version}
                        </Badge>
                      )}
                    </div>
                    <div className="text-xs text-zinc-500 capitalize">
                      {svc.running ? "Running" : "Stopped"}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  {!svc.running ? (
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 hover:bg-emerald-500/10 hover:text-emerald-400"
                      title="Start"
                      disabled={loading}
                      onClick={() => handleAction(svc.name, "start")}
                    >
                      <Play className="w-4 h-4" />
                    </Button>
                  ) : (
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 hover:bg-red-500/10 hover:text-red-400"
                      title="Stop"
                      disabled={loading}
                      onClick={() => handleAction(svc.name, "stop")}
                    >
                      <Square className="w-4 h-4" />
                    </Button>
                  )}
                  <Button
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8 hover:bg-indigo-500/10 hover:text-indigo-400"
                    title="Restart"
                    disabled={loading}
                    onClick={() => handleAction(svc.name, "restart")}
                  >
                    <RotateCw className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            ))}
            {services.length === 0 && (
              <div className="text-center py-6 text-zinc-500">
                Loading services...
              </div>
            )}
          </CardContent>
        </Card>

        {/* System Health */}
        <Card className="bg-zinc-900 border-zinc-800">
          <CardHeader>
            <CardTitle className="flex items-center text-white">
              <Activity className="w-5 h-5 mr-2 text-emerald-400" />
              System Health
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {health.length === 0 ? (
              <div className="text-center py-6 text-zinc-500">
                No health checks implemented yet.
              </div>
            ) : (
              health.map((check, i) => (
                <div
                  key={i}
                  className="flex items-center justify-between p-3 bg-zinc-950/50 rounded-lg border border-zinc-900"
                >
                  <span className="text-zinc-300 font-medium">
                    {check.name}
                  </span>
                  <Badge
                    variant={
                      check.status === "pass" ? "default" : "destructive"
                    }
                  >
                    {check.message}
                  </Badge>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
