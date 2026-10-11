import type { QueryClient } from "@tanstack/react-query";
import { api, type AccountStatus } from "@/api";

export async function refreshAccountManualPriority(
  queryClient: QueryClient,
  accountId: string,
): Promise<void> {
  const detailKey = ["account-detail", accountId];
  await Promise.all([
    queryClient.cancelQueries({ queryKey: ["accounts"], exact: true }),
    queryClient.cancelQueries({ queryKey: detailKey, exact: true }),
  ]);
  const detail = await queryClient.fetchQuery({
    queryKey: detailKey,
    queryFn: async () => {
      const current = await api.account(accountId);
      if (current.id !== accountId) {
        throw new Error("返回的账号与当前账号不一致，请刷新列表后重试");
      }
      return current;
    },
    staleTime: 0,
    retry: false,
  });
  // 详情读取期间可能启动列表轮询，旧响应不能覆盖刚确认的账号状态。
  await queryClient.cancelQueries({ queryKey: ["accounts"], exact: true });
  queryClient.setQueryData<AccountStatus[]>(["accounts"], (accounts) =>
    accounts?.map((account) => (account.id === accountId ? detail : account)),
  );
}
