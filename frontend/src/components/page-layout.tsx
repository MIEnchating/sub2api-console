import { Children, isValidElement, type ReactElement, type ReactNode } from "react";

import { PageHeading, type PageHeadingProps } from "./page-heading";

export type PageLayoutProps = {
  children: ReactNode;
  fixedContent?: boolean;
  navigation?: ReactNode;
};

export function PageLayout(props: PageLayoutProps) {
  let heading: ReactElement<PageHeadingProps> | null = null;
  const content: ReactNode[] = [];

  Children.forEach(props.children, (node) => {
    if (isValidElement<PageHeadingProps>(node) && node.type === PageHeading) {
      heading = node;
      return;
    }
    content.push(node);
  });

  return (
    <div data-slot="page-layout" className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      {heading}
      {props.navigation && (
        <div data-slot="page-navigation" className="shrink-0 px-3 pb-3 sm:px-4">
          {props.navigation}
        </div>
      )}
      <div
        data-slot="page-content"
        className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain px-3 pt-1 pb-3 sm:px-4 sm:pt-1.5 sm:pb-4"
      >
        {props.fixedContent ? (
          <div data-slot="page-workspace" className="h-full min-h-[32rem] min-w-0 sm:min-h-[28rem]">
            {content}
          </div>
        ) : (
          content
        )}
      </div>
    </div>
  );
}
