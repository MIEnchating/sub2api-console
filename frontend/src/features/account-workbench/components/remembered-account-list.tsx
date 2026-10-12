import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ContentRetry } from "@/components/content-retry";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { RememberedLogin } from "./remembered-login";
import { rememberedAccountKey } from "../constants";
import type { RememberedAccount } from "../types";
import { WorkbenchEmptyState, WorkbenchToolbar } from "./workbench-section";

const emptyAccounts: RememberedAccount[] = [];

export function RememberedAccountList(props: { onStarted?: () => void }): ReactElement {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: rememberedAccountKey,
    queryFn: ({ signal }) => api.workbenchRememberedAccounts(signal),
  });
  const [login, setLogin] = useState<RememberedAccount | null>(null);
  const pagination = useClientPagination(query.data ?? emptyAccounts);
  const [pendingDelete, setPendingDelete] = useState<RememberedAccount | null>(null);
  const remove = useMutation({
    mutationFn: (account: RememberedAccount) =>
      api.deleteWorkbenchRememberedAccount(account.id, account.revision),
    onSuccess: () => {
      setPendingDelete(null);
      void client.invalidateQueries({ queryKey: rememberedAccountKey });
    },
    onError: (error) => notifyOperationError(error, "已记住账号删除失败"),
  });
  if (query.isPending) {
    return (
      <section id="remembered-accounts" aria-label="已记住账号" className="grid gap-3">
        <Skeleton className="h-8 w-36" />
        <Skeleton className="h-28 w-full" />
      </section>
    );
  }
  if (!query.data) {
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  }
  return (
    <section
      id="remembered-accounts"
      aria-label="已记住账号"
      className="grid min-w-0 gap-3 rounded-lg border bg-card p-4"
    >
      <WorkbenchToolbar
        description="已导入账号会持续保留，可重新登录或手动删除。"
        meta={<Badge variant="secondary">{query.data.length} 个账号</Badge>}
        actions={
          <Button
            variant="outline"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" />
            刷新
          </Button>
        }
      />
      {query.data.length === 0 ? (
        <WorkbenchEmptyState
          icon={ShieldCheck}
          title="暂无已记住账号"
          description="完成账号导入后可在这里查看；邮箱登录资料可用于后续重新登录。"
        />
      ) : (
        <div className="min-w-0 overflow-x-auto rounded-lg border">
          <Table aria-label="已记住账号列表" actionColumn className="min-w-[680px]">
            <TableHeader>
              <TableRow>
                <TableHead>账号</TableHead>
                <TableHead>站点账号 ID</TableHead>
                <TableHead>登录资料</TableHead>
                <TableHead>最近导入</TableHead>
                <TableHead className="w-52">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pagination.visibleItems.map((account) => (
                <TableRow key={account.id}>
                  <TableCell className="max-w-64 whitespace-normal wrap-anywhere">
                    <strong className="font-medium">
                      {account.name || account.email || `账号 ${account.id}`}
                    </strong>
                    {account.email && account.email !== account.name && (
                      <div className="text-xs text-muted-foreground">{account.email}</div>
                    )}
                  </TableCell>
                  <TableCell className="font-mono text-xs">#{account.account_id}</TableCell>
                  <TableCell>
                    <Badge variant={account.login_available ? "success" : "warning"}>
                      {account.login_available ? "可重新登录" : "需补充登录资料"}
                    </Badge>
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-sm">
                    {new Date(account.last_imported_at).toLocaleString("zh-CN")}
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button
                        variant="outline"
                        disabled={!account.login_available || remove.isPending}
                        aria-label={`重新登录 ${account.email || account.name || account.id}`}
                        onClick={() => setLogin(account)}
                      >
                        重新登录
                      </Button>
                      <Button
                        variant="destructive"

                        aria-label={`删除 ${account.name || account.email || account.id}`}
                        disabled={remove.isPending}
                        onClick={() => setPendingDelete(account)}
                      >
                        <Trash2 aria-hidden="true" />
                        删除
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {query.data.length > 0 && (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={query.data.length}
          pageSize={pagination.pageSize}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      )}
      {login && (
        <RememberedLogin
          account={login}
          onClose={() => setLogin(null)}
          onStarted={props.onStarted}
        />
      )}
      {pendingDelete && (
        <Dialog open onOpenChange={(open) => !open && !remove.isPending && setPendingDelete(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>删除已记住账号</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <p className="text-sm">
                删除 {pendingDelete.email || pendingDelete.name}（#{pendingDelete.account_id}
                ）的已保存登录资料，后续需重新导入才能使用；线上账号保留。
              </p>
            </DialogBody>
            <DialogFooter>
              <Button
                variant="outline"
                disabled={remove.isPending}
                onClick={() => setPendingDelete(null)}
              >
                返回
              </Button>
              <Button disabled={remove.isPending} onClick={() => remove.mutate(pendingDelete)}>
                确认删除
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
