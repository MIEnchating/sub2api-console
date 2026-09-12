import { useEffect, useRef, useState } from "react";
import type { ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api";
import type { BrowserLoginInput, BrowserLoginSession } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { notifyOperationError } from "@/lib/operation-feedback";
import { BrowserSurface } from "./browser-surface";

const activeStatuses = new Set(["starting", "waiting", "verifying"]);

export function BrowserLogin(props: { host: string; disabled?: boolean }): ReactElement {
  const [open, setOpen] = useState(false);
  const [session, setSession] = useState<BrowserLoginSession | null>(null);
  const queryClient = useQueryClient();
  const activeID = useRef<string | null>(null);
  const mounted = useRef(true);
  const generation = useRef(0);
  const reported = useRef<string | null>(null);
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const query = useQuery({
    queryKey: ["browser-login", session?.id],
    queryFn: () => api.browserLogin(session!.id),
    enabled: open && session !== null,
    gcTime: 0,
    retry: false,
    refetchInterval: (value) =>
      activeStatuses.has(value.state.data?.status ?? session?.status ?? "") && !value.state.error
        ? 1000
        : false,
  });
  const value = query.data ?? session;
  const start = useMutation({
    mutationFn: async () => {
      const current = generation.current;
      const result = await api.startBrowserLogin(props.host);
      if (!mounted.current || generation.current !== current) {
        await api.cancelBrowserLogin(result.id);
        return null;
      }
      return result;
    },
    onSuccess: (result) => {
      if (result) {
        activeID.current = result.id;
        setSession(result);
      }
    },
    onError: (error) => notifyOperationError(error, "验证浏览器启动失败"),
  });
  const input = useMutation({
    gcTime: 0,
    mutationFn: (value: { id: string; input: BrowserLoginInput }) =>
      api.browserLoginInput(value.id, value.input),
    onError: (error) => notifyOperationError(error, "浏览器操作失败"),
  });
  const finish = useMutation({
    mutationFn: async (id: string) => {
      await queue.current;
      if (id !== activeID.current) return;
      return api.finishBrowserLogin(id);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["browser-login", session?.id] });
    },
    onError: (error) => notifyOperationError(error, "登录复核未开始"),
  });
  const close = (): void => {
    generation.current += 1;
    const id = activeID.current;
    activeID.current = null;
    setOpen(false);
    setSession(null);
    if (id)
      void api
        .cancelBrowserLogin(id)
        .catch((error: unknown) =>
          notifyOperationError(error, "浏览器关闭失败，会话将在到期后清理"),
        );
    queryClient.removeQueries({ queryKey: ["browser-login"] });
    start.reset();
    finish.reset();
    input.reset();
    queue.current = Promise.resolve();
  };
  useEffect(() => {
    mounted.current = true;
    const abandon = (): void => {
      const id = activeID.current;
      if (id) void api.cancelBrowserLogin(id).catch(() => undefined);
    };
    window.addEventListener("pagehide", abandon);
    return () => {
      window.removeEventListener("pagehide", abandon);
      mounted.current = false;
      generation.current += 1;
      const id = activeID.current;
      if (id) void api.cancelBrowserLogin(id).catch(() => undefined);
      queryClient.removeQueries({ queryKey: ["browser-login"] });
    };
  }, [queryClient]);
  useEffect(() => {
    if (!value || activeStatuses.has(value.status) || reported.current === value.id) return;
    reported.current = value.id;
    if (value.status === "succeeded") {
      toast.success("鉴权已复核并保存，请查看上游同步状态");
      void queryClient.invalidateQueries({ queryKey: ["upstreams"] });
      void queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] });
    } else {
      toast.error(value.message);
    }
  }, [value, queryClient]);
  const send = (payload: BrowserLoginInput): Promise<unknown> => {
    const id = activeID.current;
    const next = queue.current
      .catch(() => undefined)
      .then(() => {
        if (id !== activeID.current || !id) return;
        return input.mutateAsync({ id, input: payload });
      });
    queue.current = next;
    return next;
  };
  const waiting = value?.status === "waiting" && !finish.isPending && !query.isError;
  return (
    <>
      <Button
        type="button"
        variant="outline"
        disabled={props.disabled}
        onClick={() => {
          generation.current += 1;
          reported.current = null;
          setOpen(true);
          start.mutate();
        }}
      >
        打开浏览器手动验证
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!next) close();
        }}
      >
        <DialogContent width="wide" height="adaptive">
          <DialogHeader>
            <DialogTitle>浏览器手动验证 · {props.host}</DialogTitle>
            <DialogDescription>
              在服务器浏览器中手动完成上游验证和登录，再点击“登录完成，复核并保存”。会话有效期 15
              分钟，关闭后清除。
            </DialogDescription>
          </DialogHeader>
          {start.isPending || value?.status === "starting" ? (
            <ContentLoading label="正在启动验证浏览器" />
          ) : null}
          {value?.status === "verifying" || finish.isPending ? (
            <ContentLoading label="正在复核登录凭据" />
          ) : null}
          {value?.image ? (
            <BrowserSurface session={value} disabled={!waiting} onInput={send} />
          ) : null}
          {start.isError ? (
            <Button type="button" variant="outline" onClick={() => start.mutate()}>
              重新启动浏览器
            </Button>
          ) : null}
          {query.isError ? (
            <ContentRetry
              onRetry={() => {
                void query.refetch();
              }}
              pending={query.isFetching}
            />
          ) : null}
          {value && !activeStatuses.has(value.status) ? (
            <p role="status">
              {value.status === "succeeded" ? "鉴权已恢复" : "本次验证已结束，可关闭后重新开始。"}
            </p>
          ) : null}
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" onClick={close}>
              关闭验证
            </Button>
            <Button
              type="button"
              disabled={!waiting}
              onClick={() => {
                if (activeID.current) finish.mutate(activeID.current);
              }}
            >
              登录完成，复核并保存
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
