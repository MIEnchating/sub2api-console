import { createRoot } from "react-dom/client";
import type { ReactElement } from "react";
import type { AccountRecentResult } from "@/api";
import { AccountRecentResults } from "@/components/account-recent-results";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";

const probeResult: AccountRecentResult = {
  id: "probe-tooltip-layout",
  result: "通过",
  event_type: "healthy",
  score: 100,
  observed_at: "2026-09-14T22:12:14Z",
  latency_ms: 3303,
  failure_reason: null,
  source: "active-probe",
};

function TooltipLayout(): ReactElement {
  const surface = new URLSearchParams(window.location.search).get("surface");
  if (surface?.startsWith("hover-")) {
    const direction = surface.slice(6);
    const side =
      direction === "bottom" || direction === "left" || direction === "right" ? direction : "top";
    return (
      <div style={{ position: "fixed", left: "50%", top: direction === "flip" ? 0 : "50%" }}>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger render={<Button>查看地址</Button>} />
            <TooltipContent role="tooltip" side={side} sideOffset={16}>
              app.example.test
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    );
  }
  if (surface === "probe" || surface === "probe-failure") {
    const result =
      surface === "probe-failure"
        ? {
            ...probeResult,
            result: "失败",
            event_type: "probe_failed",
            score: 0,
            failure_reason: `上游连接失败 https://example.invalid/${"connectionrefused".repeat(8)}`,
          }
        : probeResult;
    return (
      <TooltipProvider>
        <AccountRecentResults results={[result]} />
      </TooltipProvider>
    );
  }
  let content = "模型配置已保存";
  if (surface === "name") content = "verylongunbrokenmodelname".repeat(12);
  if (surface === "url") content = `https://example.invalid/${"account".repeat(36)}`;
  if (surface === "custom")
    content = "当前账号的模型配置与分组绑定信息已更新，请检查关联分组和调度状态";
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<Button>查看说明</Button>} />
        <TooltipContent
          className={surface === "custom" || surface === "short" ? "max-w-sm" : undefined}
        >
          {content}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

createRoot(document.getElementById("tooltip-root")!).render(<TooltipLayout />);
