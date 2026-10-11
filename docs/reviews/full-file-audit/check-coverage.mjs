import { execFileSync } from "node:child_process";
import console from "node:console";
import process from "node:process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const directory = dirname(fileURLToPath(import.meta.url));
const root = resolve(directory, "../../..");
const files = execFileSync(
  "git",
  ["ls-files", "--cached", "--others", "--exclude-standard", "--deduplicate", "-z"],
  { cwd: root, maxBuffer: 16 * 1024 * 1024 },
)
  .toString()
  .split("\0")
  .filter(Boolean)
  .sort();
const reviews = new Map();
for (const ledger of readdirSync(directory).filter((name) => name.endsWith(".tsv"))) {
  for (const line of readFileSync(resolve(directory, ledger), "utf8").split("\n")) {
    const [path, hash, status, assessment] = line.split("\t");
    if (!path || !/^[a-f0-9]{64}$/.test(hash ?? "") || !["reviewed", "fixed"].includes(status))
      continue;
    const entries = reviews.get(path) ?? [];
    entries.push({ hash, ledger, assessment });
    reviews.set(path, entries);
  }
}

const records = [];
const excluded = [];
for (const path of files) {
  if (path.startsWith("docs/reviews/full-file-audit/")) continue;
  if (!existsSync(resolve(root, path))) {
    excluded.push({ path, reason: "deleted from worktree" });
    continue;
  }
  if (
    /\.(zip|tar|gz|tgz|br)$/.test(path) ||
    /^frontend\/(?:dist|dist\.previous-deploy-[^/]+|test-results|playwright-report|\.tanstack)\//.test(
      path,
    ) ||
    path === "frontend/src/routeTree.gen.ts"
  ) {
    excluded.push({ path, reason: "archive or generated build artifact" });
    continue;
  }
  const content = readFileSync(resolve(root, path));
  if (
    path.startsWith("backend/") &&
    path.endsWith(".test") &&
    content.length >= 4 &&
    content.readUInt32BE(0) === 0x7f454c46
  ) {
    excluded.push({ path, reason: "compiled Go test executable (ELF)" });
    continue;
  }
  const hash = createHash("sha256").update(content).digest("hex");
  const matching = reviews.get(path)?.find((entry) => entry.hash === hash);
  let status = "pending";
  if (matching) status = "reviewed";
  else if (reviews.has(path)) status = "changed";
  records.push({
    path,
    sha256: hash,
    bytes: content.length,
    status,
    ledger: matching?.ledger,
  });
}
const counts = {
  total: records.length,
  reviewed: 0,
  pending: 0,
  changed: 0,
  excluded: excluded.length,
};
for (const record of records) counts[record.status] += 1;
console.log(JSON.stringify(counts));
for (const record of records.filter((entry) => entry.status !== "reviewed")) {
  console.log(`${record.status}\t${record.path}`);
}
if (process.argv.includes("--write")) {
  writeFileSync(
    resolve(directory, "coverage.json"),
    JSON.stringify({ checkedAt: new Date().toISOString(), counts, excluded, records }, null, 2) +
      "\n",
  );
}
if (counts.pending || counts.changed) process.exitCode = 1;
