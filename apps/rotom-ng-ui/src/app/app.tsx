import "react-toastify/dist/ReactToastify.css";

import {
  AuthGate,
  ControllersPage,
  createAppQueryClient,
  DevicePage,
  InstanceGate,
  JobsPage,
  Layout,
  type NavItem,
  StatusPage,
  TooltipProvider,
  useActiveConfig,
  WorkersPage,
} from "@rotom-ng/base-ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { useMemo } from "react";
import { Navigate, Route, Routes } from "react-router";
import { ToastContainer } from "react-toastify";
import rotomNgIcon from "../assets/rotom-ng.png";
import { APP_VERSION } from "../version";

const queryClient = createAppQueryClient();

const baseNavItems: NavItem[] = [
  { label: "Status", path: "/" },
  { label: "Devices", path: "/devices" },
  { label: "Controllers", path: "/controllers" },
  { label: "Workers", path: "/workers" },
];

function AppContent() {
  // The active instance's config when fronted by the admin service, so the
  // nav follows the instance the operator selected rather than the service.
  const config = useActiveConfig();
  const jobsEnabled = config?.jobs?.enable === true;

  const navItems = useMemo(() => {
    if (jobsEnabled) {
      return [...baseNavItems, { label: "Jobs", path: "/jobs" }];
    }
    return baseNavItems;
  }, [jobsEnabled]);

  return (
    <TooltipProvider delayDuration={200}>
      <Layout
        appName="RotomNG"
        appIcon={rotomNgIcon}
        appVersion={APP_VERSION}
        navItems={navItems}
      >
        {/* Inside Layout so the instance picker stays reachable when the
            selected instance is down. Passes through unless the UI is fronted
            by the admin service. */}
        <InstanceGate>
          <Routes>
            <Route path="/" element={<StatusPage />} />
            <Route path="devices" element={<DevicePage />} />
            <Route path="controllers" element={<ControllersPage />} />
            <Route path="workers" element={<WorkersPage />} />
            {jobsEnabled && <Route path="jobs" element={<JobsPage />} />}
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </InstanceGate>
      </Layout>
    </TooltipProvider>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      {/* Outside AppContent so its polling queries never mount — and never
          fire a burst of 401s — before there is a session. */}
      <AuthGate appName="RotomNG" appIcon={rotomNgIcon}>
        <AppContent />
      </AuthGate>
      <ToastContainer theme="dark" />
    </QueryClientProvider>
  );
}

export default App;
