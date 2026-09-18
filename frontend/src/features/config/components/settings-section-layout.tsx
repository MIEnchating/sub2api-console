import { Children, isValidElement, type ReactElement, type ReactNode } from "react";
import { PageHeading, type PageHeadingProps } from "@/components/page-heading";
import { PageActions } from "@/components/page-actions";

export function SettingsSectionLayout(props: { children: ReactNode; fixedContent?: boolean }) {
  let heading: ReactElement<PageHeadingProps> | null = null;
  const content: ReactNode[] = [];
  Children.forEach(props.children, (node) => {
    if (isValidElement<PageHeadingProps>(node) && node.type === PageHeading) heading = node;
    else content.push(node);
  });
  const title = (heading as ReactElement<PageHeadingProps> | null)?.props;
  return (
    <section className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      {title && (
        <header className="flex shrink-0 flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold">{title.title}</h2>
            {title.description && (
              <p className="text-muted-foreground mt-1 text-sm">{title.description}</p>
            )}
          </div>
          <PageActions>{title.action}</PageActions>
        </header>
      )}
      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain">{content}</div>
    </section>
  );
}
