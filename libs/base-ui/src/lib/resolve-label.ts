export function resolveLabel(
  label: string | undefined,
  translate: (key: string, options?: { _: string }) => string,
  fallback: string,
): string {
  if (label == null || label === "") return fallback;
  // Heuristic: i18n keys are dot-namespaced. Literal labels are not.
  if (label.includes(".")) return translate(label, { _: fallback });
  return label;
}
