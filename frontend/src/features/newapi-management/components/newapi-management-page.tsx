import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CirclePlus, Pencil, RefreshCw, ServerCog, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, type NewAPIPlatform, type NewAPIRemoteSnapshot } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { notifyOperationError } from "@/lib/operation-feedback";
import { newAPIManagementViews, type NewAPIManagementView } from "../constants";
import type { NewAPIPlatformValues } from "../lib/schemas";
import { NewAPIChannelForm } from "./channel-form";
import { NewAPIGroupBindings } from "./group-bindings";
import { NewAPIModelPrices, NewAPIPriceDifferences } from "./model-prices";
import { NewAPIPlatformDialog } from "./platform-dialog";

export function NewAPIManagementPage() {
  const queryClient = useQueryClient();
  const [platformId, setPlatformId] = useState("");
  const [view, setView] = useState<NewAPIManagementView>("groups");
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

  const refresh = useMutation({
    mutationFn: () => api.refreshNewAPIPlatform(platformId),
    onSuccess: (value) => {
      setSnapshot(value);
      toast.success("New API 数据已刷新");
    },
    onError: (error) => notifyOperationError(error, "New API 数据刷新失败"),
  });

  useEffect(() => {
    setSnapshot(null);
    if (platformId) refresh.mutate();
  }, [platformId]);

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
        title="New API 管理"
        description=""
        action={
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            {workspace.data?.platforms.length ? (
              <Select
                value={platformId || null}
                onValueChange={(value) => setPlatformId(value ?? "")}
              >
                <SelectTrigger className="w-52" aria-label="New API 平台">
                  <SelectValue placeholder="选择平台" />
                </SelectTrigger>
                <SelectContent>
                  {workspace.data.platforms.map((platform) => (
                    <SelectItem key={platform.id} value={platform.id}>
                      {platform.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
            <Tooltip>
              <TooltipTrigger render={<span className="inline-flex" />}>
                <Button
                  size="icon-sm"
                  variant="outline"
                  aria-label="刷新 New API 数据"
                  disabled={!platformId || refresh.isPending}
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
            {selectedPlatform ? (
              <Tooltip>
                <TooltipTrigger render={<span className="inline-flex" />}>
                  <Button
                    size="icon-sm"
                    variant="outline"
                    aria-label="编辑 New API 平台"
                    onClick={() => {
                      setEditingPlatform(selectedPlatform);
                      setPlatformDialogOpen(true);
                    }}
                  >
                    <Pencil aria-hidden="true" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>编辑平台</TooltipContent>
              </Tooltip>
            ) : null}
            <Button
              size="sm"
              onClick={() => {
                setEditingPlatform(null);
                setPlatformDialogOpen(true);
              }}
            >
              <CirclePlus aria-hidden="true" />
              添加平台
            </Button>
          </div>
        }
      />

      {workspace.error ? (
        <QueryErrorToast error={workspace.error} fallback="New API 管理数据读取失败" />
      ) : null}

      <div className="flex h-full min-h-0 flex-col gap-3">
        {selectedPlatform ? (
          <>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-background px-3 py-2.5">
              <div className="flex min-w-0 items-center gap-3">
                <span className="bg-primary/10 text-primary flex size-8 shrink-0 items-center justify-center rounded-md">
                  <ServerCog className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-semibold">{selectedPlatform.name}</span>
                    <Badge variant="secondary">已配置</Badge>
                  </div>
                  <p className="text-muted-foreground truncate text-xs">
                    {selectedPlatform.base_url} · User ID {selectedPlatform.user_id}
                  </p>
                </div>
              </div>
              <Tooltip>
                <TooltipTrigger render={<span className="inline-flex" />}>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    className="text-destructive"
                    aria-label="删除 New API 平台"
                    onClick={() => setDeleteOpen(true)}
                  >
                    <Trash2 aria-hidden="true" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>删除平台</TooltipContent>
              </Tooltip>
            </div>

            <SegmentedControl className="w-full overflow-x-auto sm:w-fit">
              {newAPIManagementViews.map((item) => (
                <SegmentedControlItem
                  key={item.value}
                  selected={view === item.value}
                  onClick={() => setView(item.value)}
                >
                  {item.label}
                  {item.value === "differences" && snapshot?.differences.length ? (
                    <Badge variant="destructive">{snapshot.differences.length}</Badge>
                  ) : null}
                </SegmentedControlItem>
              ))}
            </SegmentedControl>

            <div className="min-h-0 flex-1 overflow-auto">
              {view === "groups" ? (
                <NewAPIGroupBindings
                  groups={snapshot?.groups ?? []}
                  localGroups={workspace.data?.local_groups ?? []}
                  bindings={workspace.data?.bindings ?? []}
                  pending={saveBindings.isPending}
                  onSave={(bindings) => saveBindings.mutate(bindings)}
                />
              ) : null}
              {view === "channels" ? (
                <NewAPIChannelForm
                  groups={workspace.data?.local_groups ?? []}
                  pending={createChannel.isPending}
                  onSubmit={(payload) => createChannel.mutate(payload)}
                />
              ) : null}
              {view === "prices" ? (
                <NewAPIModelPrices
                  models={snapshot?.models ?? []}
                  pending={savePrices.isPending}
                  onSave={(prices) => savePrices.mutate(prices)}
                />
              ) : null}
              {view === "differences" && snapshot ? (
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
              添加平台
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
