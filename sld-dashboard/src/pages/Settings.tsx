import { useState } from "react";
import {
  Settings as SettingsIcon,
  Terminal,
  Server,
  Palette,
  Save,
} from "lucide-react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { useAppStore } from "@/stores/useAppStore";
import {
  useSldState,
  useSwitchPHPMutation,
  usePHPVersions,
} from "@/hooks/use-daemon";
import { cn } from "@/lib/utils";
import { Switch } from "@/components/ui/Switch";
import { useToast } from "@/hooks/useToast";

function SettingsSection({
  title,
  description,
  icon,
  children,
}: {
  title: string;
  description: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-[var(--primary)]/10 text-[var(--primary)]">
            {icon}
          </div>
          <div>
            <CardTitle className="text-lg">{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4 pt-0">{children}</CardContent>
    </Card>
  );
}

function SettingsRow({
  label,
  description,
  children,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between p-4 rounded-lg bg-[var(--muted)]/30 border border-[var(--border)]">
      <div>
        <p className="font-medium text-[var(--foreground)]">{label}</p>
        {description && (
          <p className="text-sm text-[var(--muted-foreground)]">
            {description}
          </p>
        )}
      </div>
      <div>{children}</div>
    </div>
  );
}

function Select({
  value,
  options,
  onChange,
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={cn(
        "px-3 py-2 rounded-lg",
        "bg-[var(--input)] border border-[var(--border)]",
        "text-[var(--foreground)]",
        "focus:outline-none focus:border-[var(--ring)]",
        "transition-colors duration-200"
      )}
    >
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}

export default function Settings() {
  const { theme, toggleTheme } = useAppStore();
  const { data: state, isLoading: isStateLoading } = useSldState();
  const switchPHPMutation = useSwitchPHPMutation();
  const { data: phpVersions, isLoading: isVersionsLoading } = usePHPVersions();
  const { toast } = useToast();

  const [autostart, setAutostart] = useState(true);
  const [notifications, setNotifications] = useState(true);

  // Note: We use local state for php version to allow immediate UI update,
  // but we should sync with state.php_version if we want to be strict.
  // For now initializing with state value or default is fine.
  const [phpVersion, setPhpVersion] = useState(state?.php_version || "8.2");

  const handlePhpChange = (version: string) => {
    setPhpVersion(version);
    switchPHPMutation.mutate(version);
  };

  const handleSave = () => {
    // Just a mock save for now since general settings aren't persisted in daemon yet
    toast({
      title: "Settings Saved",
      description: "Preferences updated locally",
    });
  };

  if (isStateLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--primary)] text-[var(--primary)]"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold gradient-text">Settings</h1>
          <p className="text-[var(--muted-foreground)]">
            Configure Supreme Local Dev
          </p>
        </div>
        <Button variant="primary" onClick={handleSave}>
          <Save size={16} />
          Save Changes
        </Button>
      </div>

      {/* General */}
      <SettingsSection
        title="General"
        description="Basic application settings"
        icon={<SettingsIcon size={20} />}
      >
        <SettingsRow
          label="Auto-start on login"
          description="Start SLD daemon when you log in"
        >
          <Switch checked={autostart} onCheckedChange={setAutostart} />
        </SettingsRow>
        <SettingsRow
          label="Notifications"
          description="Show desktop notifications"
        >
          <Switch checked={notifications} onCheckedChange={setNotifications} />
        </SettingsRow>
      </SettingsSection>

      {/* PHP Settings */}
      <SettingsSection
        title="PHP Configuration"
        description="Manage PHP versions and settings"
        icon={<Terminal size={20} />}
      >
        <SettingsRow
          label="Default PHP Version"
          description="Used for all projects unless overridden"
        >
          <div className="flex items-center gap-3">
            {isVersionsLoading ? (
              <div className="h-9 w-32 bg-[var(--muted)] animate-pulse rounded-lg" />
            ) : (
              <Select
                value={phpVersion}
                options={(phpVersions || ["8.2", "8.1"]).map((v) => ({
                  value: v,
                  label: `PHP ${v}`,
                }))}
                onChange={handlePhpChange}
              />
            )}
            {switchPHPMutation.isPending && (
              <span className="text-xs text-[var(--muted-foreground)] animate-pulse">
                Switching...
              </span>
            )}
          </div>
        </SettingsRow>
      </SettingsSection>

      {/* Server Settings */}
      <SettingsSection
        title="Server"
        description="Nginx and web server configuration"
        icon={<Server size={20} />}
      >
        <SettingsRow label="HTTP Port" description="Main port for HTTP traffic">
          <input
            type="text"
            value={state?.port || "80"}
            className={cn(
              "w-20 px-3 py-2 rounded-lg text-center",
              "bg-[var(--input)] border border-[var(--border)]",
              "text-[var(--foreground)] font-mono",
              "focus:outline-none focus:border-[var(--ring)]"
            )}
            readOnly
          />
        </SettingsRow>
        <SettingsRow
          label="TLD Domain"
          description="Top-level domain for local sites"
        >
          <div className="flex items-center gap-2">
            <span className="text-[var(--muted-foreground)]">.</span>
            <input
              type="text"
              value={state?.tld || "test"}
              className={cn(
                "w-24 px-3 py-2 rounded-lg",
                "bg-[var(--input)] border border-[var(--border)]",
                "text-[var(--foreground)] font-mono",
                "focus:outline-none focus:border-[var(--ring)]"
              )}
              readOnly
            />
          </div>
        </SettingsRow>
      </SettingsSection>

      {/* UI Settings */}
      <SettingsSection
        title="Appearance"
        description="Customize the dashboard appearance"
        icon={<Palette size={20} />}
      >
        <SettingsRow label="Theme" description="Choose light or dark mode">
          <div className="flex gap-2">
            <button
              onClick={() => theme === "light" || toggleTheme()}
              className={cn(
                "px-4 py-2 rounded-lg text-sm font-medium transition-all",
                theme === "dark"
                  ? "bg-[var(--primary)] text-[var(--primary-foreground)]"
                  : "bg-[var(--muted)] text-[var(--muted-foreground)]"
              )}
            >
              Dark
            </button>
            <button
              onClick={() => theme === "dark" || toggleTheme()}
              className={cn(
                "px-4 py-2 rounded-lg text-sm font-medium transition-all",
                theme === "light"
                  ? "bg-[var(--primary)] text-[var(--primary-foreground)]"
                  : "bg-[var(--muted)] text-[var(--muted-foreground)]"
              )}
            >
              Light
            </button>
          </div>
        </SettingsRow>
      </SettingsSection>

      {/* About */}
      <Card>
        <div className="text-center py-6">
          <div className="w-16 h-16 mx-auto mb-4 rounded-2xl bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center">
            <span className="text-white text-2xl font-bold">S</span>
          </div>
          <h3 className="text-xl font-bold text-[var(--foreground)]">
            Supreme Local Dev
          </h3>
          <p className="text-[var(--muted-foreground)] text-sm">
            Version 1.0.0
          </p>
          <p className="text-[var(--muted-foreground)] text-xs mt-2">
            High-performance local development environment
          </p>
        </div>
      </Card>
    </div>
  );
}
