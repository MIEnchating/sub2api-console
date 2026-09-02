import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, Pencil, RefreshCw, ServerCog, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, type NewAPIPlatform, type NewAPIRemoteSnapshot } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { notifyOperationError } from "@/lib/operation-feedback";
import type { NewAPIManagementView } from "../constants";
import type { NewAPIPlatformValues } from "../lib/schemas";
import { NewAPIChannelForm } from "./channel-form";
import { NewAPIGroupBindings } from "./group-bindings";
import { NewAPIModelPrices, NewAPIPriceDifferences } from "./model-prices";
import { NewAPIPlatformDialog } from "./platform-dialog";

type Props = {
  view: "platform" | NewAPIManagementView;
};

const pageTitles: Record<Props["view"], string> = {
  platform: "New API 主平台",
  groups: "分组绑定",
  channels: "渠道管理",
  prices: "模型价格",
  differences: "价格差异",
};

export function newAPIViewNeedsRemoteSnapshot(view: Props["view"]): boolean {
  return view === "groups" || view === "channels" || view === "prices" || view === "differences";
}

export function NewAPIRemoteLoading(props: { label: string }) {
  return (
    <section
      className="overflow-hidden rounded-md border bg-background"
      role="status"
      aria-label={props.label}
    >
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div className="grid gap-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-3 w-16" />
        </div>
        <Skeleton className="h-7 w-24" />
      </div>
      <div className="grid gap-px bg-border">
        {Array.from({ length: 6 }, (_, index) => (
          <div
            key={index}
            className="grid h-15 grid-cols-[minmax(8rem,1fr)_7rem_minmax(12rem,1fr)_5rem] items-center gap-4 bg-background px-4"
          >
            <Skeleton className="h-3 w-32 max-w-full" />
            <Skeleton className="h-3 w-12" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-4 w-8 justify-self-center" />
          </div>
        ))}
      </div>
    </section>
  );
}

export function NewAPIPlatformDetails(props: { platform: NewAPIPlatform }) {
  return (
    <section className="grid gap-px overflow-hidden rounded-md border bg-border sm:grid-cols-2">
      <div className="bg-background p-4">
        <p className="text-muted-foreground text-xs">平台地址</p>
        <p className="mt-1 truncate text-sm font-medium">{props.platform.base_url}</p>
      </div>
      <div className="bg-background p-4">
        <p className="text-muted-foreground text-xs">User ID</p>
        <p className="mt-1 font-mono text-sm font-medium">{props.platform.user_id}</p>
      </div>
      <div className="bg-background p-4">
        <p className="text-muted-foreground text-xs">Admin Key</p>
        <p className="mt-1 text-sm font-medium">
          {props.platform.admin_key_configured ? "已配置" : "未配置"}
        </p>
      </div>
      <div className="bg-background p-4">
        <p className="text-muted-foreground text-xs">最后更新</p>
        <p className="mt-1 text-sm font-medium">
          {new Date(props.platform.updated_at).toLocaleString("zh-CN")}
        </p>
      </div>
    </section>
  );
}

export function NewAPIManagementPage(props: Props) {
  const queryClient = useQueryClient();
  const [platformId, setPlatformId] = useState("");
  const [platformDialogOpen, setPlatformDialogOpen] = useState(false);
  const [editingPlatform, setEditingPlatform] = useState<NewAPIPlatform | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [snapshot, setSnapshot] = useState<NewAPIRemoteSnapshot | null>(null);

  const workspace = useQuery({
    queryKey: ["newapi-workspace", platformId],
    queryFn: () => api.newAPIWorkspace(platformId || undefined),
    retry: false,
  });

  useEffect(() => {
    if (platformId || !workspace.data?.platforms[0]) return;
    setPlatformId(workspace.data.platforms[0].id);
  }, [platformId, workspace.data?.platforms]);

  const selectedPlatform =
    workspace.data?.platforms.find((platform) => platform.id === platformId) ?? null;
  const needsRemoteSnapshot = newAPIViewNeedsRemoteSnapshot(props.view);

  const refresh = useMutation({
    mutationFn: () => api.refreshNewAPIPlatform(platformId),
    onSuccess: (value) => {
      setSnapshot(value);
    },
    onError: (error) => notifyOperationError(error, "New API 数据刷新失败"),
  });

  useEffect(() => {
    setSnapshot(null);
    if (platformId && needsRemoteSnapshot) refresh.mutate();
  }, [needsRemoteSnapshot, platformId]);

  const savePlatform = useMutation({
    mutationFn: (values: NewAPIPlatformValues) =>
      api.saveNewAPIPlatform({
        ...(editingPlatform ? { id: editingPlatform.id } : {}),
        name: values.name.trim(),
        base_url: values.base_url.trim(),
        admin_key: values.admin_key.trim(),
        user_id: values.user_id.trim(),
      }),
    onSuccess: async (platform) => {
      setPlatformDialogOpen(false);
      setEditingPlatform(null);
      await queryClient.invalidateQueries({ queryKey: ["newapi-workspace"] });
      setPlatformId(platform.id);
      toast.success("New API 平台已保存");
    },
    onError: (error) => notifyOperationError(error, "New API 平台保存失败"),
  });

  const deletePlatform = useMutation({
    mutationFn: () => api.deleteNewAPIPlatform(platformId),
    onSuccess: async () => {
      setDeleteOpen(false);
      setSnapshot(null);
      setPlatformId("");
      await queryClient.invalidateQueries({ queryKey: ["newapi-workspace"] });
      toast.success("New API 平台已删除");
    },
    onError: (error) => notifyOperationError(error, "New API 平台删除失败"),
  });

  const saveBindings = useMutation({
    mutationFn: (bindings: Parameters<typeof api.saveNewAPIGroupBindings>[1]) =>
      api.saveNewAPIGroupBindings(platformId, bindings),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["newapi-workspace", platformId] });
      const refreshed = await api.refreshNewAPIPlatform(platformId);
      setSnapshot(refreshed);
      toast.success("分组绑定已保存");
    },
    onError: (error) => notifyOperationError(error, "分组绑定保存失败"),
  });

  const createChannel = useMutation({
    mutationFn: (payload: Parameters<typeof api.createNewAPIChannel>[1]) =>
      api.createNewAPIChannel(platformId, payload),
    onSuccess: () => toast.success("New API 渠道已创建"),
    onError: (error) => notifyOperationError(error, "New API 渠道创建失败"),
  });

  const createChannelKey = useMutation({
    mutationFn: (payload: Parameters<typeof api.createNewAPIChannelKey>[1]) =>
      api.createNewAPIChannelKey(platformId, payload),
    onError: (error) => notifyOperationError(error, "Sub2API 密钥创建失败"),
  });

  const fetchChannelModels = useMutation({
    mutationFn: async (payload: Parameters<typeof api.fetchNewAPIChannelModels>[1]) => {
      const result = await api.fetchNewAPIChannelModels(platformId, payload);
      return result.models;
    },
  });

  const savePrices = useMutation({
    mutationFn: (prices: Parameters<typeof api.saveNewAPIModelPrices>[1]) =>
      api.saveNewAPIModelPrices(platformId, prices),
    onSuccess: (value) => {
      setSnapshot(value);
      toast.success("模型价格已更新");
    },
    onError: (error) => notifyOperationError(error, "模型价格更新失败"),
  });

  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="运营管理"
        title={pageTitles[props.view]}
        description=""
        action={
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            {selectedPlatform ? (
              <>
                {needsRemoteSnapshot ? (
                  <Tooltip>
                    <TooltipTrigger render={<span className="inline-flex" />}>
                      <Button
                        size="icon-sm"
                        variant="outline"
                        aria-label="刷新 New API 数据"
                        disabled={refresh.isPending}
                        onClick={() => refresh.mutate()}
                      >
                        <RefreshCw
                          className={refresh.isPending ? "animate-spin" : ""}
                          aria-hidden="true"
                        />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>刷新</TooltipContent>
                  </Tooltip>
                ) : null}
                <Tooltip>
                  <TooltipTrigger render={<span className="inline-flex" />}>
                    <Button
                      size="icon-sm"
                      variant="outline"
                      aria-label="编辑 New API 主平台"
                      onClick={() => {
                        setEditingPlatform(selectedPlatform);
                        setPlatformDialogOpen(true);
                      }}
                    >
                      <Pencil aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>编辑主平台</TooltipContent>
                </Tooltip>
                {props.view === "platform" ? (
                  <Tooltip>
                    <TooltipTrigger render={<span className="inline-flex" />}>
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        className="text-destructive"
                        aria-label="删除 New API 主平台"
                        onClick={() => setDeleteOpen(true)}
                      >
                        <Trash2 aria-hidden="true" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>删除主平台</TooltipContent>
                  </Tooltip>
                ) : null}
              </>
            ) : (
              <Button
                size="sm"
                onClick={() => {
                  setEditingPlatform(null);
                  setPlatformDialogOpen(true);
                }}
              >
                <CirclePlus aria-hidden="true" />
                配置主平台
              </Button>
            )}
          </div>
        }
      />

      {workspace.error ? (
        <QueryErrorToast error={workspace.error} fallback="New API 管理数据读取失败" />
      ) : null}

      <div className="flex h-full min-h-0 flex-col gap-3">
        {workspace.isLoading ? (
          <NewAPIRemoteLoading label="正在加载 New API 主平台" />
        ) : selectedPlatform ? (
          <>
            <div className="min-h-0 flex-1 overflow-auto">
              {needsRemoteSnapshot && refresh.isPending && snapshot === null ? (
                <NewAPIRemoteLoading label={`正在加载${pageTitles[props.view]}`} />
              ) : null}
              {props.view === "platform" ? (
                <NewAPIPlatformDetails platform={selectedPlatform} />
              ) : null}
              {props.view === "groups" && !(refresh.isPending && snapshot === null) ? (
                <NewAPIGroupBindings
                  groups={snapshot?.groups ?? []}
                  localGroups={workspace.data?.local_groups ?? []}
                  bindings={workspace.data?.bindings ?? []}
                  pending={saveBindings.isPending}
                  onSave={(bindings) => saveBindings.mutate(bindings)}
                />
              ) : null}
              {props.view === "channels" && !(refresh.isPending && snapshot === null) ? (
                <NewAPIChannelForm
                  groups={workspace.data?.local_groups ?? []}
                  newAPIGroups={snapshot?.groups ?? []}
                  sub2APIBaseURL={workspace.data?.sub2api_base_url ?? ""}
                  pending={createChannel.isPending}
                  creatingKey={createChannelKey.isPending}
                  fetchingModels={fetchChannelModels.isPending}
                  onCreateKey={(payload) => createChannelKey.mutateAsync(payload)}
                  onFetchModels={(payload) => fetchChannelModels.mutateAsync(payload)}
                  onSubmit={async (payload) => {
                    await createChannel.mutateAsync(payload);
                  }}
                />
              ) : null}
              {props.view === "prices" && !(refresh.isPending && snapshot === null) ? (
                <NewAPIModelPrices
                  models={snapshot?.models ?? []}
                  pending={savePrices.isPending}
                  onSave={(prices) => savePrices.mutate(prices)}
                />
              ) : null}
              {props.view === "differences" && snapshot ? (
                <NewAPIPriceDifferences snapshot={snapshot} />
              ) : null}
            </div>
          </>
        ) : (
          <div className="text-muted-foreground flex min-h-72 flex-1 flex-col items-center justify-center gap-3 rounded-md border border-dashed bg-background px-6 text-center text-sm">
            <ServerCog className="size-10 opacity-45" aria-hidden="true" />
            <span>尚未配置 New API 平台</span>
            <Button
              size="sm"
              onClick={() => {
                setEditingPlatform(null);
                setPlatformDialogOpen(true);
              }}
            >
              <CirclePlus aria-hidden="true" />
              配置主平台
            </Button>
          </div>
        )}
      </div>

      <NewAPIPlatformDialog
        open={platformDialogOpen}
        platform={editingPlatform}
        pending={savePlatform.isPending}
        onOpenChange={setPlatformDialogOpen}
        onSubmit={(values) => savePlatform.mutate(values)}
      />
      <ConfirmActionDialog
        open={deleteOpen}
        title="删除 New API 平台"
        description={`将删除 ${selectedPlatform?.name ?? "当前平台"} 的管理凭据和全部分组绑定。`}
        confirmLabel="删除平台"
        pending={deletePlatform.isPending}
        onOpenChange={setDeleteOpen}
        onConfirm={() => deletePlatform.mutate()}
      />
    </PageLayout>
  );
}
