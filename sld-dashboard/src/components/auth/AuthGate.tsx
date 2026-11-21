import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Check, Copy, KeyRound, Loader2 } from "lucide-react";
import { v2 } from "@/api/v2";
import { UNAUTHORIZED_EVENT } from "@/lib/fetch-interceptor";
import { queryClient } from "@/lib/react-query";
import { Button } from "@/components/ui/Button";

type State = "checking" | "signed-out" | "signed-in" | "unreachable";

/**
 * Shows the sign-in screen until the browser has a session, and again
 * whenever an API call answers 401 (expired session, server restarted).
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State>("checking");
  const [tokenCommand, setTokenCommand] = useState("");

  const check = () => {
    v2.authStatus()
      .then((s) => {
        if (s.token_command) setTokenCommand(s.token_command);
        setState(s.authenticated ? "signed-in" : "signed-out");
      })
      .catch(() => setState("unreachable"));
  };

  useEffect(() => {
    // A sign-in link (`sld token`) carries the token in the URL fragment, which
    // never reaches the server or its logs. Drop it from the address bar at
    // once, then sign in with it.
    const linkToken = new URLSearchParams(window.location.hash.slice(1)).get("token");
    if (linkToken) {
      window.history.replaceState(null, "", window.location.pathname + window.location.search);
      v2.login(linkToken)
        .then(() => queryClient.invalidateQueries())
        .catch(() => undefined)
        .finally(check);
    } else {
      check();
    }
    const onUnauthorized = () => setState("signed-out");
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
  }, []);

  if (state === "signed-in") return <>{children}</>;

  return (
    <div className="min-h-screen bg-[var(--background)] flex items-center justify-center p-4">
      <div className="w-full max-w-sm rounded-xl border border-[var(--border)] bg-[var(--card)] p-6 shadow-sm">
        <div className="flex items-center gap-3 mb-6">
          <div className="w-10 h-10 rounded-lg bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center">
            <span className="text-white font-bold">S</span>
          </div>
          <div>
            <h1 className="text-lg font-semibold text-[var(--foreground)]">Supreme Local Dev</h1>
            <p className="text-xs text-[var(--muted-foreground)]">Sign in to manage your projects</p>
          </div>
        </div>
        {state === "checking" && (
          <div className="flex items-center gap-2 text-sm text-[var(--muted-foreground)]">
            <Loader2 className="animate-spin" size={16} /> Connecting…
          </div>
        )}
        {state === "unreachable" && (
          <div className="space-y-3 text-sm">
            <p className="text-red-400">Cannot reach the SLD control plane.</p>
            <p className="text-[var(--muted-foreground)]">Check that the SLD container is running, then retry.</p>
            <Button variant="secondary" onClick={check}>Retry</Button>
          </div>
        )}
        {state === "signed-out" && <SignInForm tokenCommand={tokenCommand} onSignedIn={() => { queryClient.invalidateQueries(); setState("signed-in"); }} />}
      </div>
    </div>
  );
}

function SignInForm({ tokenCommand, onSignedIn }: { tokenCommand: string; onSignedIn: () => void }) {
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const command = tokenCommand || "docker exec <sld-container> sld token";

  const copyCommand = () => {
    navigator.clipboard?.writeText(command).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }).catch(() => undefined);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await v2.login(token.trim());
      onSignedIn();
    } catch {
      setError("That token was not accepted.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <label className="block text-sm font-medium text-[var(--foreground)]" htmlFor="sld-token">
        Admin token
      </label>
      <div className="relative">
        <KeyRound size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)]" />
        <input
          id="sld-token"
          type="password"
          autoComplete="current-password"
          autoFocus
          value={token}
          onChange={(e) => setToken(e.target.value)}
          className="w-full h-10 pl-9 pr-3 rounded-lg border border-[var(--border)] bg-[var(--background)] text-sm text-[var(--foreground)] focus:outline-none focus:ring-2 focus:ring-[var(--ring)]"
          placeholder="Paste your admin token"
        />
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}
      <Button type="submit" className="w-full" loading={busy} disabled={!token.trim()}>
        Sign in
      </Button>
      <div className="space-y-1.5 text-xs text-[var(--muted-foreground)] leading-relaxed">
        <p>Lost it? Run this on your machine for a new token and a sign-in link:</p>
        <div className="flex items-center gap-2">
          <code className="flex-1 min-w-0 truncate px-2 py-1 rounded bg-[var(--secondary)] text-[var(--foreground)]" title={command}>
            {command}
          </code>
          <button
            type="button"
            onClick={copyCommand}
            className="shrink-0 inline-flex items-center gap-1 px-2 py-1 rounded border border-[var(--border)] hover:bg-[var(--secondary)]"
            aria-label="Copy command"
          >
            {copied ? <Check size={12} /> : <Copy size={12} />}
            {copied ? "Copied" : "Copy"}
          </button>
        </div>
      </div>
    </form>
  );
}
