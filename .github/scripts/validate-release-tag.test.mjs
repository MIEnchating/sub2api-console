import assert from "node:assert/strict";
import test from "node:test";

import { validateReleaseNotesContent, validateReleaseTag } from "./validate-release-tag.mjs";

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
  for (const tag of ["v2026.02.30", "v1.0.0", "v2026.08.28-1", "2026.08.28"]) {
    assert.throws(() => validateReleaseTag(tag, { checkSequence: false }));
  }
});

test("requires exactly five complete release-note sections in order", () => {
  assert.doesNotThrow(() => validateReleaseNotesContent(completeNotes));
  assert.throws(() => validateReleaseNotesContent(completeNotes.replace("- 首个版本", "")));
  assert.throws(() =>
    validateReleaseNotesContent(`${completeNotes}\n## 部署说明\n\n- 重启服务\n`),
  );
});
