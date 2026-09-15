import type { ReactElement } from "react";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { Skeleton } from "@/components/ui/skeleton";
import { SegmentedControl } from "@/components/ui/segmented-control";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const catalogColumns = ["w-56", "w-40", "w-44", "w-40", "w-48"];
const revenueColumns = [
  "w-56",
  "w-40",
  "w-48",
  "w-28",
  "w-28",
  "w-32",
  "w-20",
  "w-28",
  "w-32",
  "w-28",
  "w-28",
  "min-w-56",
];

function PricingTableSkeleton(props: {
  label: string;
  columns: string[];
  tableClassName: string;
  testId?: string;
}): ReactElement {
  return (
    <DataTablePanel
      role="status"
      aria-label={props.label}
      aria-busy="true"
      className="h-full flex-1"
      data-testid={props.testId}
    >
      <span className="sr-only">{props.label}</span>
      <div aria-hidden="true" className="flex min-h-0 flex-1 flex-col">
        <Table className={props.tableClassName} containerClassName="min-h-0 flex-1 overflow-auto">
          <TableHeader data-slot="skeleton-table-header">
            <TableRow>
              {props.columns.map((column, index) => (
                <TableHead key={index} className={column}>
                  <Skeleton className="h-4 w-20 max-w-full" />
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {[0, 1, 2, 3, 4, 5].map((row) => (
              <TableRow key={row}>
                {props.columns.map((_, column) => (
                  <TableCell key={column} overflowTooltip={false}>
                    <Skeleton className="h-4 w-3/4" />
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div
          data-slot="skeleton-pagination"
          className="flex shrink-0 items-center justify-end gap-2 border-t px-3 py-2.5 sm:px-4 sm:py-3"
        >
          <Skeleton className="h-4 w-12" />
          <Skeleton className="h-8 w-16" />
          <Skeleton className="size-8" />
          <Skeleton className="size-8" />
        </div>
      </div>
    </DataTablePanel>
  );
}

export function PricingCatalogSkeleton(): ReactElement {
  return (
    <PricingTableSkeleton
      label="正在读取价格目录"
      columns={catalogColumns}
      tableClassName="min-w-[960px]"
      testId="pricing-loading"
    />
  );
}

export function RevenueReportSkeleton(): ReactElement {
  return (
    <PricingTableSkeleton
      label="正在读取最近一次分析"
      columns={revenueColumns}
      tableClassName="min-w-[100rem] table-fixed"
    />
  );
}

export function RevenueNavigationSkeleton(): ReactElement {
  return (
    <SegmentedControl
      aria-hidden="true"
      data-testid="revenue-navigation-skeleton"
      className="grid w-full grid-cols-3 sm:w-fit"
    >
      <Skeleton className="h-8 min-w-0 sm:w-24" />
      <Skeleton className="h-8 min-w-0 sm:w-24" />
      <Skeleton className="h-8 min-w-0 sm:w-28" />
    </SegmentedControl>
  );
}
