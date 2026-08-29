import {
  describeSearchableFields,
  type SearchableField,
} from "../search/searchable";
import type { Worker } from "../types";

/**
 * Every field the workers search box looks at.
 *
 * "status" is here so that typing `active` or `inactive` narrows the table --
 * it was in the old inner filter's field list but unreachable, because the
 * outer filter ran first over device fields and threw every device away before
 * the inner one saw the term.
 *
 * `session.controller.id` is deliberately absent. It was searched in three
 * places and populated in none, so the field only ever widened the haystack
 * with undefined.
 */
export const WORKER_SEARCH_FIELDS: SearchableField<Worker>[] = [
  { label: "origin", get: (worker) => worker.origin },
  { label: "worker ID", get: (worker) => worker.id },
  { label: "device ID", get: (worker) => worker.device_id },
  { label: "version", get: (worker) => worker.version_code },
  { label: "user agent", get: (worker) => worker.user_agent },
  { label: "platform", get: (worker) => worker.platform },
  { label: "status", get: (worker) => (worker.is_in_use ? "active" : "idle") },
];

export const WORKER_SEARCH_FIELD_LABELS =
  describeSearchableFields(WORKER_SEARCH_FIELDS);

/**
 * Worker ids are only unique within their device, and these tables flatten
 * across devices, so the same id can appear on several rows at once. Keying
 * rows on the id alone collides in React and makes every namesake expand and
 * collapse together.
 */
export const workerRowKey = (worker: Worker, index: number): string =>
  worker.id ? `${worker.device_id}/${worker.id}` : `worker-${index}`;
