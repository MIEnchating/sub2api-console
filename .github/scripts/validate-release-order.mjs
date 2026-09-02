import { validateReleaseOrder } from "./validate-release-tag.mjs";

const [previousTag, candidateTag, previousRevision, candidateRevision] = process.argv.slice(2);
if (!previousTag || !candidateTag || !previousRevision || !candidateRevision) {
  throw new Error(
    "Previous tag, candidate tag, previous revision, and candidate revision are required",
  );
}
validateReleaseOrder(previousTag, candidateTag, previousRevision, candidateRevision);
console.log(`Validated release order: ${previousTag} -> ${candidateTag}`);
