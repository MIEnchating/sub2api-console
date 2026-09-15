import { useEffect, useState, type ReactElement } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { WorkbenchSecurity } from "./workbench-security";

export function WorkbenchOAuthSecurity(props: {
  sourceId: string;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const [expired, setExpired] = useState(false);
  const source = useQuery({
    queryKey: ["account-workbench", "security-source", props.sourceId],
    queryFn: (context) => api.workbenchSecuritySource(props.sourceId, context.signal),
    gcTime: 0,
    retry: false,
  });
  const sourceId = props.sourceId;
  useEffect(
    () => () => {
      const key = ["account-workbench", "security-source", sourceId];
      void client.cancelQueries({ queryKey: key });
      client.removeQueries({ queryKey: key });
    },
    [client, sourceId],
  );
  useEffect(() => {
    if (!source.data) return;
    const remaining = Date.parse(source.data.expires_at) - Date.now();
    const timer = setTimeout(
      () => setExpired(true),
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [source.data]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>授权账号安全设置</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {source.isPending ? <ContentLoading label="正在核对授权账号身份" /> : null}
          {!source.isPending && !source.data ? (
            <ContentRetry pending={source.isFetching} onRetry={() => void source.refetch()} />
          ) : null}
          {source.data ? (
            <div className="grid min-w-0 gap-4">
              <dl aria-label="授权账号身份" className="grid min-w-0 gap-2 text-sm wrap-anywhere">
                <div>
                  <dt className="text-muted-foreground">邮箱</dt>
                  <dd>{source.data.email}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">官方用户 ID</dt>
                  <dd>{source.data.user_id}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">工作区 ID</dt>
                  <dd>{source.data.workspace_id}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">授权有效期</dt>
                  <dd>{new Date(source.data.expires_at).toLocaleString("zh-CN")}</dd>
                </div>
              </dl>
              {expired ? (
                <p role="status" className="text-sm">
                  授权已到期，请重新授权
                </p>
              ) : null}
              <WorkbenchSecurity source={source.data} disabled={expired || source.isError} />
            </div>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            返回授权结果
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
