import { Skeleton } from "./ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./ui/table";

const candidateColumns = ["上游分组", "介绍", "倍率", "绑定到本地分组", "已有绑定", "操作"];

export function OnboardingSelectionSkeleton(props: {
  fillAvailableHeight: boolean;
  groupLocked: boolean;
}) {
  return (
    <div
      aria-label="正在获取上游信息"
      role="status"
      className={
        props.fillAvailableHeight
          ? "grid min-h-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-4 overflow-hidden"
          : "grid gap-4"
      }
    >
      <div
        className="grid grid-cols-2 divide-x rounded-lg border lg:grid-cols-6"
        data-onboarding-skeleton="summary"
      >
        {Array.from({ length: 6 }, (_, index) => (
          <div className="flex min-w-0 items-center justify-between gap-3 px-3 py-2.5" key={index}>
            <Skeleton className="h-4 w-14 shrink-0" />
            <Skeleton className="h-4 w-16" />
          </div>
        ))}
      </div>

      {props.groupLocked ? (
        <div
          className="grid grid-cols-2 divide-x rounded-lg border"
          data-onboarding-skeleton="locked-group"
        >
          {Array.from({ length: 2 }, (_, index) => (
            <div
              className="flex min-w-0 items-center justify-between gap-3 px-3 py-2.5"
              key={index}
            >
              <Skeleton className="h-4 w-16 shrink-0" />
              <Skeleton className="h-4 w-20" />
            </div>
          ))}
        </div>
      ) : (
        <Table
          aria-label="正在加载上游分组"
          className="min-w-[1180px]"
          containerClassName={
            props.fillAvailableHeight
              ? "min-h-0 overflow-auto rounded-lg border"
              : "max-h-[32rem] overflow-auto rounded-lg border"
          }
          data-onboarding-skeleton="groups"
        >
          <TableHeader>
            <TableRow>
              {candidateColumns.map((column) => (
                <TableHead key={column}>{column}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {Array.from({ length: 6 }, (_, row) => (
              <TableRow aria-label="正在加载分组" key={row}>
                {candidateColumns.map((column, columnIndex) => (
                  <TableCell key={column}>
                    <Skeleton className={columnIndex === 1 ? "h-4 w-full" : "h-4 w-3/4"} />
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <div
        className={props.groupLocked ? "grid gap-4 sm:grid-cols-3" : "flex items-end gap-3"}
        data-onboarding-skeleton="form"
      >
        {Array.from({ length: props.groupLocked ? 3 : 1 }, (_, index) => (
          <div className="grid gap-2" key={index}>
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-9 w-64" />
          </div>
        ))}
        {!props.groupLocked ? <Skeleton className="ml-auto h-9 w-36" /> : null}
      </div>
      {props.groupLocked ? (
        <div className="flex justify-end" data-onboarding-skeleton="action">
          <Skeleton className="h-9 w-24" />
        </div>
      ) : null}
    </div>
  );
}
