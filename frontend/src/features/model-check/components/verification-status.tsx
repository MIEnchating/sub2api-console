import type { ReactElement } from "react";
import { cn } from "@/lib/utils";
import { publicReferenceSummary } from "../lib/public-reference-summary";
import { VerificationStateBadge } from "./verification-state";
import {
  verificationCapabilityLabels,
  publicReferenceLabels,
  verificationIdentityLabels,
  verificationReferenceLabels,
  verificationFingerprintLabels,
  verificationIdentityLegacyExplanation,
} from "../constants";
import {
  verificationRecord as record,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";

export function VerificationCapabilityStatus(props: { status: string }): ReactElement {
  return (
    <VerificationStateBadge
      status={props.status}
      label={
        verificationCapabilityLabels[props.status] ?? verificationCapabilityLabels.inconclusive
      }
    />
  );
}

export function VerificationIdentityStatus(props: { result: VerificationRecord }): ReactElement {
  const publicReference = record(props.result.public_reference);
  const reference = record(props.result.reference);
  const status = text(reference.status) || text(record(props.result.authenticity).status);
  const fingerprint = record(props.result.fingerprint);
  const hidden =
    status === "not_run" || status === "incomplete" || status === "insufficient_evidence";
  const label =
    verificationReferenceLabels[status] ?? verificationIdentityLabels[status] ?? "接口信息未记录";
  return (
    <>
      {publicReference.version ? (
        <VerificationStateBadge
          status={text(publicReference.status)}
          label={
            publicReferenceLabels[text(publicReference.status)] ??
            publicReferenceLabels.inconclusive
          }
        />
      ) : null}
      {!hidden ? <VerificationStateBadge status={status} label={label} /> : null}
      {fingerprint.status ? (
        <VerificationStateBadge
          status={text(fingerprint.status)}
          label={verificationFingerprintLabels[text(fingerprint.status)] ?? "指纹待判定"}
        />
      ) : null}
    </>
  );
}

export function VerificationIdentitySummary(props: {
  result: VerificationRecord;
  prominent?: boolean;
}): ReactElement {
  const authenticity = record(props.result.authenticity);
  const known = Object.hasOwn(verificationIdentityLabels, text(authenticity.status));
  return (
    <p
      className={cn(
        "leading-relaxed wrap-anywhere",
        props.prominent ? "text-sm" : "text-muted-foreground text-xs",
      )}
    >
      {identitySummary(props.result, known)}
    </p>
  );
}

function identitySummary(result: VerificationRecord, known: boolean): string {
  const publicReference = record(result.public_reference);
  if (publicReference.version) return publicReferenceSummary(result);
  const reference = record(result.reference);
  if (["matched", "mismatch", "not_applicable"].includes(text(reference.status)))
    return text(reference.summary);
  if (
    reference.status === "not_run" ||
    reference.status === "incomplete" ||
    record(result.authenticity).status === "insufficient_evidence"
  )
    return "";
  if (known && text(record(result.authenticity).summary))
    return text(record(result.authenticity).summary);
  return verificationIdentityLegacyExplanation;
}
