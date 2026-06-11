export interface Job {
  id: string;
  description: string;
  exec: string;
}

export interface JobInstance {
  id?: number;
  job_id: string;
  started_at_ms: number;
  finished_at_ms?: number;
  device_id: string;
  device_origin?: string;
  result: string;
  status: string;
}

export type Jobs = Job[];
export type JobInstances = JobInstance[];
