import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight } from "lucide-react";
import { motion } from "motion/react";
import { useCallback, useState } from "react";
import { Collapse } from "../anim/collapse";
import { MagneticButton } from "../anim/magnetic-button";
import { Button } from "../components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
import {
  AESTHETIC_CARD,
  AESTHETIC_CARD_HEADER,
  AESTHETIC_DANGER_BUTTON,
} from "../lib/aesthetic";
import {
  jobInstancesQuery,
  jobsQuery,
  statusQuery,
} from "../lib/query-options";
import { JobsStatusesTable } from "./jobs-statuses-table";
import { JobsTable } from "./jobs-table";

export const JobsPage = () => {
  const queryClient = useQueryClient();
  const [isJobsExpanded, setIsJobsExpanded] = useState(true);
  const [isStatusesExpanded, setIsStatusesExpanded] = useState(true);

  const {
    isLoading: areJobInstancesLoading,
    refetch: refreshJobInstances,
    error: jobInstancesError,
    data: jobInstancesResponse,
    isSuccess: jobInstancesSuccess,
  } = useQuery(jobInstancesQuery());

  const { data: devices, refetch: refetchDevices } = useQuery({
    ...statusQuery(),
    select: (s) => s.devices,
  });

  const {
    isLoading: areJobsLoading,
    isFetching: areJobsFetching,
    error: jobsError,
    data: jobsResponse,
    isSuccess: jobsSuccess,
  } = useQuery(jobsQuery());

  const clearStatusesMutation = useMutation({
    mutationFn: async () => {
      const cancel = new AbortController();
      const timer = setTimeout(() => cancel.abort(), 5000);
      try {
        const res = await fetch("/api/job-instance/-/clear", {
          method: "PUT",
          signal: cancel.signal,
        });
        if (!res.ok) throw new Error(`Clear failed: ${res.status}`);
        return res.json();
      } finally {
        clearTimeout(timer);
      }
    },
    onSettled: () => queryClient.invalidateQueries(jobInstancesQuery()),
  });

  const handleClearStatuses = useCallback(() => {
    clearStatusesMutation.mutate();
  }, [clearStatusesMutation]);

  const reloadJobsMutation = useMutation({
    mutationFn: async () => {
      const cancel = new AbortController();
      const timer = setTimeout(() => cancel.abort(), 5000);
      try {
        const res = await fetch("/api/job/-/reload", {
          method: "PUT",
          signal: cancel.signal,
        });
        if (!res.ok) {
          let errorMsg = "An unknown error occurred reloading jobs";
          try {
            const ct = res.headers.get("content-type");
            if (ct?.includes("application/json")) {
              const data = await res.json();
              if (data.error) errorMsg = data.error;
            }
          } catch {}
          throw new Error(errorMsg);
        }
        return res.json();
      } finally {
        clearTimeout(timer);
      }
    },
    onError: (err) => {
      alert(
        err instanceof Error
          ? err.message
          : "An unknown error occurred reloading jobs",
      );
    },
    onSettled: () => queryClient.invalidateQueries(jobsQuery()),
  });

  const handleReloadJobs = useCallback(() => {
    reloadJobsMutation.mutate();
  }, [reloadJobsMutation]);

  if (jobsError || !jobsSuccess) {
    return (
      <div className="p-4">
        <p className="text-destructive">
          An error has occurred: {jobsError?.message}
        </p>
      </div>
    );
  }

  if (jobInstancesError || !jobInstancesSuccess) {
    return (
      <div className="p-4">
        <p className="text-destructive">
          An error has occurred: {jobInstancesError?.message}
        </p>
      </div>
    );
  }

  const jobs = jobsResponse?.jobs || [];
  const jobInstances = jobInstancesResponse?.instances || [];

  return (
    <div className="flex flex-col">
      <div>
        <h1 className="text-3xl font-bold tracking-tight gradient-text mb-4">
          Jobs
        </h1>
      </div>
      <div className="mb-4">
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45, ease: [0.16, 1, 0.3, 1] }}
        >
          <Card className={AESTHETIC_CARD}>
            <CardHeader className={AESTHETIC_CARD_HEADER}>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setIsJobsExpanded(!isJobsExpanded)}
                  aria-label={
                    isJobsExpanded ? "Collapse jobs table" : "Expand jobs table"
                  }
                >
                  {isJobsExpanded ? (
                    <ChevronDown className="size-4" />
                  ) : (
                    <ChevronRight className="size-4" />
                  )}
                </Button>
                <CardTitle className="text-xl">List ({jobs.length})</CardTitle>
              </div>
              <MagneticButton
                size="sm"
                onClick={handleReloadJobs}
                disabled={
                  reloadJobsMutation.isPending ||
                  areJobsLoading ||
                  areJobsFetching
                }
              >
                {reloadJobsMutation.isPending ? "Reloading..." : "Reload"}
              </MagneticButton>
            </CardHeader>
            <Collapse open={isJobsExpanded}>
              <CardContent className="p-4">
                <JobsTable
                  devices={devices}
                  isLoading={areJobsLoading || areJobsFetching}
                  jobs={jobs}
                  refetchDevices={refetchDevices}
                  refreshJobInstances={refreshJobInstances}
                />
              </CardContent>
            </Collapse>
          </Card>
        </motion.div>
      </div>
      <div>
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45, delay: 0.05, ease: [0.16, 1, 0.3, 1] }}
        >
          <Card className={AESTHETIC_CARD}>
            <CardHeader className={AESTHETIC_CARD_HEADER}>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setIsStatusesExpanded(!isStatusesExpanded)}
                  aria-label={
                    isStatusesExpanded
                      ? "Collapse statuses table"
                      : "Expand statuses table"
                  }
                >
                  {isStatusesExpanded ? (
                    <ChevronDown className="size-4" />
                  ) : (
                    <ChevronRight className="size-4" />
                  )}
                </Button>
                <CardTitle className="text-xl">
                  Statuses ({jobInstances.length})
                </CardTitle>
              </div>
              <MagneticButton
                size="sm"
                onClick={handleClearStatuses}
                disabled={clearStatusesMutation.isPending}
                className={AESTHETIC_DANGER_BUTTON}
              >
                {clearStatusesMutation.isPending ? "Removing..." : "Remove"}
              </MagneticButton>
            </CardHeader>
            <Collapse open={isStatusesExpanded}>
              <CardContent className="p-4">
                <JobsStatusesTable
                  isLoading={areJobInstancesLoading}
                  jobs={jobs}
                  jobInstances={jobInstances}
                />
              </CardContent>
            </Collapse>
          </Card>
        </motion.div>
      </div>
    </div>
  );
};
