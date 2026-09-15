import type { ReactElement, ReactNode } from "react";

export function AlertPolicyLayout(props: {
  detection: ReactNode;
  children: ReactNode;
}): ReactElement {
  return (
    <div data-slot="alert-policy-columns" className="grid min-w-0 items-start gap-4 lg:grid-cols-2">
      {props.detection}
      <div className="grid min-w-0 gap-4" data-slot="alert-policy-notification-column">
        {props.children}
      </div>
    </div>
  );
}
