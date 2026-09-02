import assert from "node:assert/strict";
import test from "node:test";

import {
  validateReleaseNotesContent,
  validateReleaseOrder,
  validateReleaseTag,
} from "./validate-release-tag.mjs";

const completeNotes = `## 版本概览

- 首个版本

## 新增功能

- 无

## 功能改进

- 无

## 问题修复

- 无

## 移除与调整

- 无
`;

test("accepts the calendar version and ordered same-day revision formats", () => {
  assert.deepEqual(validateReleaseTag("v2026.08.28", { checkSequence: false }), {
    baseTag: "v2026.08.28",
    revision: undefined,
  });
  assert.deepEqual(validateReleaseTag("v2026.08.28-2", { checkSequence: false }), {
    baseTag: "v2026.08.28",
    revision: 2,
  });
});

test("rejects invalid dates, semantic versions, and the forbidden -1 revision", () => {
  for (const tag of [
    "v2026.02.30",
    "v1.0.0",
    "v2026.08.28-1",
    "v2026.08.28-9007199254740992",
    "2026.08.28",
  ]) {
    assert.throws(() => validateReleaseTag(tag, { checkSequence: false }));
  }
});

test("release order accepts forward progress and rejects rollback or tag reuse", () => {
  assert.doesNotThrow(() =>
    validateReleaseOrder("v2026.08.31", "v2026.09.01", "revision-a", "revision-b"),
  );
  assert.doesNotThrow(() =>
    validateReleaseOrder("v2026.09.01", "v2026.09.01-2", "revision-a", "revision-b"),
  );
  assert.throws(() =>
    validateReleaseOrder("v2026.09.02", "v2026.09.01", "revision-a", "revision-b"),
  );
  assert.throws(() =>
    validateReleaseOrder("v2026.09.01-2", "v2026.09.01", "revision-a", "revision-b"),
  );
  assert.throws(() =>
    validateReleaseOrder("v2026.09.01", "v2026.09.01", "revision-a", "revision-b"),
  );
});

test("requires exactly five complete release-note sections in order", () => {
  assert.doesNotThrow(() => validateReleaseNotesContent(completeNotes));
  assert.throws(() => validateReleaseNotesContent(completeNotes.replace("- 首个版本", "")));
  assert.throws(() =>
    validateReleaseNotesContent(
      completeNotes.replace(
        "## 新增功能\n\n- 无",
        "## 新增功能\n\n<!-- 没有内容时填写“- 无”。 -->\n\n- 无",
      ),
    ),
  );
  assert.throws(() =>
    validateReleaseNotesContent(`${completeNotes}\n## 部署说明\n\n- 重启服务\n`),
  );
});
