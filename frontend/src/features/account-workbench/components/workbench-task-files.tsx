import type { ReactElement } from "react";
import type { Task } from "@/api";
import { workbenchResultItems } from "../lib/task-results";
import { WorkbenchExportArtifacts } from "./workbench-export-artifacts";

export function WorkbenchTaskFiles(props: { task: Task }): ReactElement | null {
  const reports = workbenchResultItems(props.task.result).flatMap((item) =>
    item.report && typeof item.report.artifact_id === "string" ? [item.report] : [],
  );
  if (!reports.length) return null;
  const managed = reports
    .filter((item) => item.scope !== "local-export")
    .map((item) => String(item.artifact_id));
  const local = reports
    .filter((item) => item.scope === "local-export")
    .map((item) => String(item.artifact_id));
  return (
    <>
      {managed.length > 0 && <WorkbenchExportArtifacts artifactIds={managed} />}
      {local.length > 0 && <WorkbenchExportArtifacts scope="local-export" artifactIds={local} />}
    </>
  );
}
