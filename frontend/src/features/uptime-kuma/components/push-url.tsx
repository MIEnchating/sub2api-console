import { useMutation } from "@tanstack/react-query";
import { Eye, EyeOff } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { notifyOperationError } from "@/lib/operation-feedback";

export function PushURL(props: { id: number; revision: string }) {
  const query = useMutation({
    mutationFn: () => api.kumaPushURL(props.id, props.revision),
    gcTime: 0,
    onError: (error) => notifyOperationError(error, "上报地址读取失败"),
  });
  return (
    <div className="grid gap-2">
      <Button
        variant="outline"
        disabled={query.isPending}
        onClick={() => (query.data ? query.reset() : query.mutate())}
      >
        {query.data ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
        {query.data ? "隐藏上报地址" : "显示上报地址"}
      </Button>
      {query.data && (
        <Input
          aria-label="Push 上报地址"
          value={query.data.url}
          readOnly
          onFocus={(event) => event.currentTarget.select()}
        />
      )}
    </div>
  );
}
