import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { AppShell } from "@/components/layout/AppShell";
import ProjectsV2 from "@/pages/ProjectsV2";
import Database from "@/pages/Database";
import Setup from "@/pages/Setup";

import { ErrorBoundary } from "@/components/common/ErrorBoundary";
import { AuthGate } from "@/components/auth/AuthGate";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/react-query";

// Pages of the V1 host daemon (Domains, Plugins, X-Ray, Share, Doctor, host
// Logs/Settings) are not routed: their backends do not exist in the SLD image.
function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <AuthGate>
          <BrowserRouter>
            <Routes>
              <Route element={<AppShell />}>
                <Route path="/" element={<Navigate to="/projects" replace />} />
                <Route path="/projects" element={<ProjectsV2 />} />
                <Route path="/database" element={<Database />} />
                <Route path="/setup" element={<Setup />} />
                <Route path="*" element={<Navigate to="/projects" replace />} />
              </Route>
            </Routes>
          </BrowserRouter>
        </AuthGate>
      </ErrorBoundary>
    </QueryClientProvider>
  );
}

export default App;
