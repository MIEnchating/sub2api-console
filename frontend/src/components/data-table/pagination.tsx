import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn, getPageNumbers } from "@/lib/utils";

const defaultPageSizes = [10, 20, 30, 50, 100];

export const paginationPageSizeSearchable = false;

export type DataTablePaginationProps = {
  currentPage: number;
  totalPages: number;
  totalItems: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  pageSizes?: number[];
};

export function DataTablePagination(props: DataTablePaginationProps) {
  const pageNumbers = getPageNumbers(props.currentPage, props.totalPages);
  const pageSizes = props.pageSizes ?? defaultPageSizes;
  const pageSizeItems = pageSizes.map((pageSize) => ({
    value: `${pageSize}`,
    label: pageSize,
  }));

  return (
    <div className="border-t px-3 py-2.5 sm:px-4 sm:py-3">
      <div
        className={cn("@container/pagination flex min-w-0 items-center justify-end overflow-clip")}
      >
        <div className="flex min-w-0 shrink-0 items-center gap-2 @xl/pagination:gap-3">
          <div className="flex shrink-0 items-baseline gap-1.5 text-xs font-medium whitespace-nowrap sm:text-sm">
            <span className="text-muted-foreground/80">共</span>
            <span className="text-foreground tabular-nums">
              {props.totalItems.toLocaleString()}
            </span>
          </div>

          <div className="flex shrink-0 items-center gap-1.5 @lg/pagination:gap-2">
            <span className="text-muted-foreground/80 hidden text-sm font-medium whitespace-nowrap @2xl/pagination:block">
              每页行数
            </span>
            <Select
              items={pageSizeItems}
              value={`${props.pageSize}`}
              onValueChange={(value) => props.onPageSizeChange(Number(value))}
            >
              <SelectTrigger
                appearance="classic"
                className="text-foreground h-8 w-[64px] font-medium tabular-nums sm:w-[70px]"
              >
                <SelectValue placeholder={props.pageSize} />
              </SelectTrigger>
              <SelectContent
                appearance="classic"
                side="top"
                alignItemWithTrigger={false}
                searchable={paginationPageSizeSearchable}
              >
                <SelectGroup>
                  {pageSizes.map((pageSize) => (
                    <SelectItem appearance="classic" key={pageSize} value={`${pageSize}`}>
                      {pageSize}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          <div className="flex min-w-0 shrink-0 items-center gap-1 @lg/pagination:gap-1.5 @xl/pagination:gap-2">
            <PaginationButton
              label="转到第一页"
              hiddenOnCompact
              disabled={props.currentPage <= 1}
              onClick={() => props.onPageChange(1)}
            >
              <ChevronsLeft />
            </PaginationButton>
            <PaginationButton
              label="转到上一页"
              disabled={props.currentPage <= 1}
              onClick={() => props.onPageChange(props.currentPage - 1)}
            >
              <ChevronLeft />
            </PaginationButton>

            {pageNumbers.map((pageNumber, index) =>
              typeof pageNumber === "string" ? (
                <span
                  key={`ellipsis:${index}`}
                  className="text-muted-foreground/60 px-0.5 text-sm @lg/pagination:px-1"
                >
                  ...
                </span>
              ) : (
                <Button
                  key={pageNumber}
                  variant={props.currentPage === pageNumber ? "default" : "outline"}
                  className={cn(
                    "h-8 min-w-8 px-2 tabular-nums",
                    props.currentPage === pageNumber
                      ? "font-semibold"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                  aria-label={`转到第 ${pageNumber} 页`}
                  aria-current={props.currentPage === pageNumber ? "page" : undefined}
                  onClick={() => props.onPageChange(pageNumber)}
                >
                  {pageNumber}
                </Button>
              ),
            )}

            <PaginationButton
              label="转到下一页"
              disabled={props.currentPage >= props.totalPages}
              onClick={() => props.onPageChange(props.currentPage + 1)}
            >
              <ChevronRight />
            </PaginationButton>
            <PaginationButton
              label="转到最后一页"
              hiddenOnCompact
              disabled={props.currentPage >= props.totalPages}
              onClick={() => props.onPageChange(props.totalPages)}
            >
              <ChevronsRight />
            </PaginationButton>
          </div>
        </div>
      </div>
    </div>
  );
}

function PaginationButton(props: {
  label: string;
  disabled: boolean;
  onClick: () => void;
  children: React.ReactNode;
  hiddenOnCompact?: boolean;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            className={cn("inline-flex", props.hiddenOnCompact && "@max-lg/pagination:hidden")}
          />
        }
      >
        <Button
          variant="outline"
          className="text-muted-foreground hover:text-foreground disabled:text-muted-foreground/50 size-8 p-0"
          aria-label={props.label}
          disabled={props.disabled}
          onClick={props.onClick}
        >
          {props.children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{props.label}</TooltipContent>
    </Tooltip>
  );
}
