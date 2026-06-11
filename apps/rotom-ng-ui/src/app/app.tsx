import "react-toastify/dist/ReactToastify.css";

import { APP_VERSION } from "../version";
import {
  ControllersPage,
  DevicePage,
  JobsPage,
  Layout,
  type NavItem,
  StatusPage,
  TooltipProvider,
  WorkersPage,
  createAppQueryClient,
  useConfig,
} from "@rotom-ng/base-ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { useMemo } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { ToastContainer } from "react-toastify";
import rotomNgIcon from "../assets/rotom-ng.png";

const queryClient = createAppQueryClient();

const baseNavItems: NavItem[] = [
  { label: "Status", path: "/" },
  { label: "Devices", path: "/devices" },
  { label: "Controllers", path: "/controllers" },
  { label: "Workers", path: "/workers" },
];

function AppContent() {
  const { data: configData } = useConfig();
  const jobsEnabled = configData?.config?.jobs?.enable === true;

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
        <Routes>
          <Route path="/" element={<StatusPage />} />
          <Route path="devices" element={<DevicePage />} />
          <Route path="controllers" element={<ControllersPage />} />
          <Route path="workers" element={<WorkersPage />} />
          {jobsEnabled && <Route path="jobs" element={<JobsPage />} />}
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Layout>
    </TooltipProvider>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppContent />
      <ToastContainer theme="dark" />
    </QueryClientProvider>
  );
}

export default App;
