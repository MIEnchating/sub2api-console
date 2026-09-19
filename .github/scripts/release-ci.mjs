import { appendFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

// Only the latest push run of this repository's CI workflow for the exact main
// commit is evidence. PR runs and an older successful retry are not evidence.
export async function findReusableCI(options) {
  const request = options.fetch ?? globalThis.fetch;
  const sleep = options.sleep ?? ((ms) => new Promise((resolve) => setTimeout(resolve, ms)));
  const attempts = options.attempts ?? 61;
  const url = new URL(
    `https://api.github.com/repos/${options.repository}/actions/workflows/ci.yml/runs`,
  );
  url.search = new URLSearchParams({
    head_sha: options.sha,
    branch: "main",
    event: "push",
    per_page: "100",
  });
  for (let attempt = 0; attempt < attempts; attempt++) {
    try {
      const response = await request(url, {
        headers: {
          Authorization: `Bearer ${options.token}`,
          Accept: "application/vnd.github+json",
        },
        signal: AbortSignal.timeout(15_000),
      });
      if (!response.ok) return null;
      const data = await response.json();
      if (!Array.isArray(data.workflow_runs)) return null;
      const runs = data.workflow_runs
        .filter(
          (run) =>
            run.head_sha === options.sha &&
            run.head_branch === "main" &&
            run.event === "push" &&
            run.repository?.full_name === options.repository &&
            Number.isSafeInteger(run.id),
        )
        .sort((a, b) => b.id - a.id);
      const latest = runs[0];
      if (!latest) return null;
      if (latest.status === "completed") return latest.conclusion === "success" ? latest.id : null;
      if (!["queued", "in_progress", "waiting", "pending", "requested"].includes(latest.status))
        return null;
      if (attempt + 1 < attempts) await sleep(30_000);
    } catch {
      return null;
    }
  }
  return null;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const runID = await findReusableCI({
    repository: process.env.GITHUB_REPOSITORY,
    sha: process.env.GITHUB_SHA,
    token: process.env.GH_TOKEN,
  });
  appendFileSync(process.env.GITHUB_OUTPUT, `reuse=${runID !== null}\n`);
  const summary =
    runID === null
      ? "未找到可复用的成功 CI，执行完整发布检查。"
      : `复用同一提交的成功 CI：https://github.com/${process.env.GITHUB_REPOSITORY}/actions/runs/${runID}`;
  console.log(summary);
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${summary}\n`);
}
