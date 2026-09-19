import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UpstreamConcurrencyMode } from "../../constants";
import { UpstreamConcurrencyPolicyCard } from "../upstream-concurrency-policy-card";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function Editor(props: {
  pending?: boolean;
  failed?: boolean;
  onRetry?: () => void;
  initialIDs?: string[];
  initialMode?: UpstreamConcurrencyMode;
}) {
  const [mode, setMode] = useState<UpstreamConcurrencyMode>(props.initialMode ?? "selected");
  const [ids, setIDs] = useState(props.initialIDs ?? []);
  return (
    <UpstreamConcurrencyPolicyCard
      enabled
      onEnabledChange={() => undefined}
      accountMode={mode}
      accountIDs={ids}
      onAccountModeChange={setMode}
      onAccountIDsChange={setIDs}
      accounts={[
        { id: "41", name: "账号甲", upstream_type: "sub2api", groups: ["codex"] },
        { id: "42", name: "账号乙", upstream_type: "newapi", groups: [] },
      ]}
      upstreamIDs={ids}
      onUpstreamIDsChange={setIDs}
      upstreams={[
        { upstream_id: "upstream-a", name: "上游甲", host: "a.example", upstream_type: "sub2api" },
        {
          upstream_id: "upstream-a",
          name: "上游甲",
          host: "alias.example",
          upstream_type: "sub2api",
        },
        { upstream_id: "upstream-b", name: "上游乙", host: "b.example", upstream_type: "newapi" },
      ]}
      upstreamsPending={props.pending}
      upstreamsFailed={props.failed}
      onRetryUpstreams={props.onRetry}
      accountsPending={props.pending}
      accountsFailed={props.failed}
      onRetryAccounts={props.onRetry}
    />
  );
}

describe("共享并发指定账号范围", () => {
  it("指定账号只列出 Sub2API 账号，并可选中和取消", async () => {
    const user = userEvent.setup();
    render(<Editor />);
    expect(screen.getByText(/尚未选择账号/)).toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "参与共享并发分配的账号" }));
    expect(screen.queryByRole("option", { name: /账号乙/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: /账号甲/ }));
    expect(screen.queryByText(/尚未选择账号/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: /账号甲/ }));
    expect(screen.getByText(/尚未选择账号/)).toBeInTheDocument();
  });
  it("切换全部账号再切回指定账号保留原选择", async () => {
    const user = userEvent.setup();
    render(<Editor initialIDs={["41"]} />);
    await user.click(screen.getByRole("combobox", { name: "共享并发分配范围" }));
    await user.click(screen.getByRole("option", { name: "全部符合条件的账号" }));
    expect(
      screen.queryByRole("combobox", { name: "参与共享并发分配的账号" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "共享并发分配范围" }));
    await user.click(await screen.findByRole("option", { name: "指定账号" }));
    expect(screen.getByText(/账号甲（#41）/)).toBeInTheDocument();
  });
  it("账号读取期间禁用选择并保留已配置 ID", () => {
    render(<Editor pending initialIDs={["999"]} />);
    expect(screen.getByRole("combobox", { name: "参与共享并发分配的账号" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("status", { name: "正在加载账号" })).toBeInTheDocument();
    expect(screen.getByText(/账号 #999/)).toBeInTheDocument();
  });
  it("读取失败提供重试并阻止清空已有选择", async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    render(<Editor failed onRetry={retry} initialIDs={["41"]} />);
    expect(screen.getByRole("combobox", { name: "参与共享并发分配的账号" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    await user.click(screen.getByRole("button", { name: "重新读取" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});

describe("共享并发指定上游范围", () => {
  it("选择指定上游时只列出 Sub2API 上游，别名按稳定 ID 去重并可取消", async () => {
    const user = userEvent.setup();
    render(<Editor />);
    await user.click(screen.getByRole("combobox", { name: "共享并发分配范围" }));
    await user.click(screen.getByRole("option", { name: "指定上游" }));
    expect(screen.getByText(/尚未选择上游/)).toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "参与共享并发分配的上游" }));
    expect(screen.queryByRole("option", { name: /上游乙/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole("option", { name: /上游甲/ })).toHaveLength(1);
    await user.click(screen.getByRole("option", { name: /上游甲/ }));
    expect(screen.queryByText(/尚未选择上游/)).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    const selector = screen.getByRole("combobox", { name: "参与共享并发分配的上游" });
    expect(selector).toHaveAttribute("aria-expanded", "false");
    screen.getByRole("button", { name: "移除" }).focus();
    await user.keyboard("{Enter}");
    expect(screen.getByText(/尚未选择上游/)).toBeInTheDocument();
  });
  it("上游加载时保留已配置 ID 并禁用选择", () => {
    render(<Editor initialMode="upstreams" pending initialIDs={["missing-upstream"]} />);
    expect(screen.getByRole("combobox", { name: "参与共享并发分配的上游" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("status", { name: "正在加载上游" })).toBeInTheDocument();
    expect(screen.getByText(/上游 missing-upstream/)).toBeInTheDocument();
  });
  it("上游读取失败允许重试并保留原选择", async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    render(<Editor initialMode="upstreams" failed onRetry={retry} initialIDs={["upstream-a"]} />);
    expect(screen.getByRole("combobox", { name: "参与共享并发分配的上游" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    await user.click(screen.getByRole("button", { name: "重新读取" }));
    expect(retry).toHaveBeenCalledOnce();
    expect(screen.getByText(/上游甲/)).toBeInTheDocument();
  });
});
