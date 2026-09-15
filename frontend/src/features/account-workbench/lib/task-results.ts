export type WorkbenchResultItem = {
  position: number;
  index: number | null;
  name: string;
  status: string;
  message: string;
  accountId: string;
  templateName?: string;
  report: Record<string, unknown> | null;
};

export function workbenchResultItems(result: Record<string, unknown>): WorkbenchResultItem[] {
  if (!Array.isArray(result.items)) return [];
  return result.items.flatMap((value: unknown, position: number) => {
    if (typeof value !== "object" || value === null) return [];
    const item = value as Record<string, unknown>;
    const index =
      typeof item.index === "number" && Number.isInteger(item.index) && item.index >= 0
        ? item.index
        : null;
    let report: Record<string, unknown> | null = null;
    if (typeof item.report === "object" && item.report !== null && !Array.isArray(item.report))
      report = item.report as Record<string, unknown>;
    return [
      {
        position,
        index,
        name: typeof item.name === "string" ? item.name : `第 ${(index ?? position) + 1} 项`,
        status: typeof item.status === "string" ? item.status : "",
        message: typeof item.message === "string" ? item.message : "",
        accountId: typeof item.account_id === "string" ? item.account_id : "",
        templateName: typeof item.template_name === "string" ? item.template_name : "",
        report,
      },
    ];
  });
}

export function resultCanRetry(item: WorkbenchResultItem): boolean {
  return item.index !== null && item.status !== "succeeded" && item.status !== "skipped";
}
