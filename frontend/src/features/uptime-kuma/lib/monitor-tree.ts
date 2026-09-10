import type { KumaMonitor } from "@/api";
import { monitorStatus } from "../constants";

export type MonitorTreeRow = {
  monitor: KumaMonitor;
  ancestors: KumaMonitor[];
  hasChildren: boolean;
  parentLabel: string;
};

export function buildMonitorTree(monitors: KumaMonitor[]): MonitorTreeRow[] {
  const byID = new Map(monitors.filter((item) => item.id > 0).map((item) => [item.id, item]));
  const children = new Map<number, KumaMonitor[]>();
  const roots: KumaMonitor[] = [];
  for (const monitor of monitors) {
    if (monitor.parent && monitor.parent !== monitor.id && byID.has(monitor.parent)) {
      const siblings = children.get(monitor.parent) ?? [];
      siblings.push(monitor);
      children.set(monitor.parent, siblings);
    } else roots.push(monitor);
  }
  const rows: MonitorTreeRow[] = [];
  const visited = new Set<string>();
  const visit = (root: KumaMonitor): void => {
    const stack: { monitor: KumaMonitor; ancestors: KumaMonitor[] }[] = [
      { monitor: root, ancestors: [] },
    ];
    while (stack.length) {
      const entry = stack.pop()!;
      if (visited.has(entry.monitor.key)) continue;
      visited.add(entry.monitor.key);
      const descendants = children.get(entry.monitor.id) ?? [];
      let parentLabel = "未分组";
      if (entry.ancestors.length)
        parentLabel = entry.ancestors.map((item) => item.name).join(" / ");
      else if (entry.monitor.parent) parentLabel = `分组 #${entry.monitor.parent}`;
      rows.push({ ...entry, hasChildren: descendants.length > 0, parentLabel });
      for (const monitor of [...descendants].reverse())
        stack.push({ monitor, ancestors: [...entry.ancestors, entry.monitor] });
    }
  };
  roots.forEach(visit);
  // Malformed cycles or missing parents must never make monitors disappear.
  monitors.forEach(visit);
  return rows;
}

export function filterMonitorTree(
  rows: MonitorTreeRow[],
  options: {
    search: string;
    status: string;
    group: string;
    management: boolean;
    collapsed: ReadonlySet<string>;
  },
): MonitorTreeRow[] {
  const search = options.search.trim().toLocaleLowerCase();
  const filtering = !!search || options.status !== "all" || options.group !== "all";
  if (!filtering)
    return rows.filter((row) => !row.ancestors.some((item) => options.collapsed.has(item.key)));
  const included = new Set<string>();
  for (const row of rows) {
    const monitor = row.monitor;
    const text =
      `${monitor.name} ${monitor.target ?? monitor.url} ${row.ancestors.map((item) => item.name).join(" ")}`.toLocaleLowerCase();
    const matchesGroup =
      options.group === "all" ||
      (options.group === "ungrouped" && !monitor.parent && monitor.type !== "group") ||
      String(monitor.id) === options.group ||
      row.ancestors.some((item) => String(item.id) === options.group);
    if (
      !text.includes(search) ||
      !matchesGroup ||
      (options.status !== "all" && monitorStatus(monitor, options.management) !== options.status)
    )
      continue;
    included.add(monitor.key);
    row.ancestors.forEach((item) => included.add(item.key));
  }
  return rows.filter((row) => included.has(row.monitor.key));
}
