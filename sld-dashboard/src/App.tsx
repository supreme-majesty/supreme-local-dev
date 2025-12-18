import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { AppShell } from "@/components/layout/AppShell";
import Dashboard from "@/pages/Dashboard";
import Projects from "@/pages/Projects";
import Domains from "@/pages/Domains";
import Plugins from "@/pages/Plugins";
import Settings from "@/pages/Settings";
import Doctor from "@/pages/Doctor";
import XRay from "@/pages/XRay";
import Share from "@/pages/Share";

import { ErrorBoundary } from "@/components/common/ErrorBoundary";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/react-query";

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <BrowserRouter>
          <Routes>
            <Route element={<AppShell />}>
              <Route path="/" element={<Dashboard />} />
              <Route path="/projects" element={<Projects />} />
              <Route path="/domains" element={<Domains />} />
              <Route path="/plugins" element={<Plugins />} />
              <Route path="/xray" element={<XRay />} />
              <Route path="/share" element={<Share />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="/doctor" element={<Doctor />} />

              {/* Fallback */}
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </ErrorBoundary>
    </QueryClientProvider>
  );
}

export default App;
