import { createRoot } from "react-dom/client";
import { useState, type ReactElement } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogBody,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";
import { MultiSelect } from "@/components/multi-select";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";

const longName = "long-model-name-".repeat(25);

function Surfaces(): ReactElement {
  const surface = new URLSearchParams(window.location.search).get("surface");
  const [selection, setSelection] = useState(["long"]);
  const [retry, setRetry] = useState(false);
  if (surface === "scroll-dialog" || surface === "short-dialog")
    return (
      <Dialog open>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认影响范围</DialogTitle>
          </DialogHeader>
          <DialogBody>
            {Array.from({ length: surface === "scroll-dialog" ? 40 : 1 }, (_, index) => (
              <p key={index} className="py-2">
                账号 {index + 1} 的模型配置将被更新
              </p>
            ))}
          </DialogBody>
          <DialogFooter>
            <Button>确认更新</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  if (surface === "dialog")
    return (
      <Dialog open>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>批量更新确认</DialogTitle>
          </DialogHeader>
          <p>当前选择 20 个账号</p>
          <DialogFooter>
            <Button variant="outline">取消操作并返回账号列表</Button>
            <Button variant="outline">重新读取当前账号的影响范围</Button>
            <Button>确认更新全部已选账号的模型配置</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  if (surface === "tooltip")
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger render={<Button>查看模型说明</Button>} />
          <TooltipContent>{longName}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  if (surface === "select")
    return (
      <div style={{ width: 200, maxWidth: "100%" }}>
        <MultiSelect
          ariaLabel="选择模型"
          options={[{ label: longName, value: "long" }]}
          selected={selection}
          onChange={setSelection}
        />
      </div>
    );
  if (surface === "retry")
    return (
      <section aria-label="内容状态">
        <Button onClick={() => setRetry(!retry)}>切换状态</Button>
        {retry ? (
          <ContentRetry onRetry={() => setRetry(false)} />
        ) : (
          <ContentLoading label="正在读取配置" />
        )}
      </section>
    );
  return (
    <div style={{ width: 200, maxWidth: "100%" }}>
      <Badge>模型同步成功</Badge>
      <Badge>{longName}</Badge>
    </div>
  );
}

createRoot(document.getElementById("audit-root")!).render(<Surfaces />);
