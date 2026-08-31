import type * as React from "react";

export type PageHeadingProps = {
  eyebrow: string;
  title: string;
  description: string;
  action?: React.ReactNode;
};

export function PageHeading(props: PageHeadingProps) {
  return (
    <div data-slot="page-heading" className="shrink-0 px-3 pt-3 pb-2.5 sm:px-4 sm:pt-5 sm:pb-3">
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 sm:gap-x-4">
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-base font-bold tracking-tight sm:text-lg">{props.title}</h2>
        </div>
        {props.action && (
          <div className="flex w-full min-w-0 flex-wrap items-center justify-end gap-2 sm:w-auto sm:shrink-0 sm:gap-x-4">
            {props.action}
          </div>
        )}
      </div>
    </div>
  );
}
