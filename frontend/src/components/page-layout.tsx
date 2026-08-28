import { Children, isValidElement, type ReactElement, type ReactNode } from "react";

import { PageHeading, type PageHeadingProps } from "./page-heading";

export type PageLayoutProps = {
  children: ReactNode;
  fixedContent?: boolean;
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
    <main data-slot="page-layout" className="flex min-h-0 flex-1 flex-col overflow-hidden">
      {heading}
      <div
        data-slot="page-content"
        className={
          props.fixedContent
            ? "min-h-0 flex-1 overflow-hidden px-3 pt-1 pb-3 sm:px-4 sm:pt-1.5 sm:pb-4"
            : "min-h-0 flex-1 overflow-auto px-3 pt-1 pb-3 sm:px-4 sm:pt-1.5 sm:pb-4"
        }
      >
        {content}
      </div>
    </main>
  );
}
