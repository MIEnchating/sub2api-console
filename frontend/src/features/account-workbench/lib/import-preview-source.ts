import type { ImportValues } from "./import-schema";

export function matchesImportPreviewSource(current: ImportValues, source: ImportValues): boolean {
  const keys = Object.keys(current) as (keyof ImportValues)[];
  return (
    keys.length === Object.keys(source).length && keys.every((key) => current[key] === source[key])
  );
}
