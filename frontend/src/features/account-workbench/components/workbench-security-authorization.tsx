import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation } from "@tanstack/react-query";
import { LogIn } from "lucide-react";
import { api, type WorkbenchOAuthSession } from "@/api";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";

export function WorkbenchSecurityAuthorization(props: {
  securityId: string;
  expiresAt: string;
  disabled?: boolean;
  onOAuth?: (session: WorkbenchOAuthSession) => void;
}): ReactElement | null {
  const generation = useRef(0);
  const mounted = useRef(false);
  const [now, setNow] = useState(Date.now);
  const expires = Date.parse(props.expiresAt);
  const expired = !(expires > now);
  useEffect(() => {
    mounted.current = true;
    const release = (): void => {
      generation.current += 1;
    };
    window.addEventListener("pagehide", release);
    return () => {
      mounted.current = false;
      release();
      window.removeEventListener("pagehide", release);
    };
  }, [props.securityId, props.expiresAt, props.disabled]);
  useEffect(() => {
    const timer = setTimeout(() => setNow(Date.now()), Math.max(0, expires - Date.now()) || 0);
    return () => clearTimeout(timer);
  }, [expires]);
  const authorize = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      if (!props.onOAuth || props.disabled || !mounted.current || !(expires > Date.now())) return;
      const requested = generation.current;
      const session = await api.authorizeWorkbenchSecurity(props.securityId);
      if (!mounted.current || requested !== generation.current || !(expires > Date.now())) {
        await api.cancelWorkbenchOAuth(session.id);
        return;
      }
      props.onOAuth(session);
    },
    onError: (error) => {
      if (mounted.current) notifyOperationError(error, "新授权启动失败，请核对安全任务后重试");
    },
  });
  if (!props.onOAuth) return null;
  return (
    <Button
      disabled={props.disabled || expired || authorize.isPending}
      onClick={() => {
        if (!authorize.isPending) authorize.mutate();
      }}
    >
      <LogIn aria-hidden="true" />
      {authorize.isPending ? "正在发起新授权" : "确认发起新的 OAuth 授权"}
    </Button>
  );
}
