import type { ReactElement } from "react";
import type { WorkbenchTab } from "../constants";
import { AccountList } from "./account-list";
import { TemplateList } from "./template-list";
import { ImportPanel } from "./import-panel";
import { RunList } from "./run-list";
import { MaintenancePanel } from "./maintenance-panel";

export function AccountWorkbenchPage(props: {
  tab: WorkbenchTab;
  onStarted: () => void;
}): ReactElement {
  switch (props.tab) {
    case "accounts":
      return <AccountList />;
    case "templates":
      return <TemplateList />;
    case "records":
      return <RunList />;
    case "maintenance":
      return <MaintenancePanel />;
    default:
      return <ImportPanel onStarted={props.onStarted} />;
  }
}
