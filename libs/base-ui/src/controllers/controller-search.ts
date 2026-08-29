import {
  describeSearchableFields,
  type SearchableField,
} from "../search/searchable";
import type { Controller } from "../types";

/**
 * Every field the controllers search box looks at.
 *
 * UUID is included because it is what a support conversation quotes, and it was
 * previously the one identifier on the row that could not be searched for.
 */
export const CONTROLLER_SEARCH_FIELDS: SearchableField<Controller>[] = [
  { label: "controller ID", get: (controller) => controller.id },
  { label: "UUID", get: (controller) => controller.uuid },
  { label: "user agent", get: (controller) => controller.user_agent },
  { label: "worker ID", get: (controller) => controller.worker_id },
];

export const CONTROLLER_SEARCH_FIELD_LABELS = describeSearchableFields(
  CONTROLLER_SEARCH_FIELDS,
);
