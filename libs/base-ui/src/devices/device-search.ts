import {
  describeSearchableFields,
  type SearchableField,
} from "../search/searchable";
import type { Device } from "../types";

/**
 * Every field the devices search box looks at. The labels are shown to the
 * reader when a search comes back empty, so they read as column names rather
 * than as property paths.
 */
export const DEVICE_SEARCH_FIELDS: SearchableField<Device>[] = [
  { label: "origin", get: (device) => device.origin },
  { label: "device ID", get: (device) => device.id },
  { label: "version", get: (device) => device.version },
  { label: "public IP", get: (device) => device.public_ip },
];

export const DEVICE_SEARCH_FIELD_LABELS =
  describeSearchableFields(DEVICE_SEARCH_FIELDS);
