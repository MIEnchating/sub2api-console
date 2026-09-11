import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { ProbeStep } from "../../hooks/use-onboarding-probe-task";
import { ProbeTaskTimeline } from "../probe-task-timeline";

afterEach(cleanup);

function completedStep(stage: string): ProbeStep {
  return {
    stage,
    status: "succeeded",
    started_at: "2026-09-11T00:00:00Z",
    finished_at: "2026-09-11T00:00:01Z",
  };
}

it("后端成功读取已有 Key 时只展示读取，不展示临时 Key 创建或复用", () => {
  render(
    <ProbeTaskTimeline
      steps={[completedStep("credential"), completedStep("read_key"), completedStep("models")]}
    />,
  );

  const timeline = within(screen.getByRole("list", { name: "探活过程" }));
  expect(timeline.getByText("读取已有上游 Key")).toBeInTheDocument();
  expect(timeline.queryByText("创建临时上游 Key")).not.toBeInTheDocument();
  expect(timeline.queryByText("复用已准备的临时 Key")).not.toBeInTheDocument();
  expect(timeline.queryByText(/读取已有 Key 或创建临时 Key/)).not.toBeInTheDocument();
});

it("后端创建临时 Key 时只展示创建，不展示读取已有 Key", () => {
  render(
    <ProbeTaskTimeline
      steps={[completedStep("credential"), completedStep("create_key"), completedStep("models")]}
    />,
  );

  const timeline = within(screen.getByRole("list", { name: "探活过程" }));
  expect(timeline.getByText("创建临时上游 Key")).toBeInTheDocument();
  expect(timeline.queryByText("读取已有上游 Key")).not.toBeInTheDocument();
  expect(timeline.queryByText(/读取已有 Key 或创建临时 Key/)).not.toBeInTheDocument();
});

it("Key 来源尚未确认时只展示校验阶段，不提前显示读取或创建", () => {
  render(
    <ProbeTaskTimeline
      steps={[{ ...completedStep("credential"), status: "running", finished_at: undefined }]}
    />,
  );

  const timeline = within(screen.getByRole("list", { name: "探活过程" }));
  expect(timeline.getByText("校验上游分组与鉴权")).toBeInTheDocument();
  expect(timeline.getByText("进行中")).toBeInTheDocument();
  expect(timeline.queryByText("读取已有上游 Key")).not.toBeInTheDocument();
  expect(timeline.queryByText("创建临时上游 Key")).not.toBeInTheDocument();
});
