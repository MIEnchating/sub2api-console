import type { CaptchaChallenge, Task } from "../api";

export function captchaChallengeFromTask(task: Task | undefined): CaptchaChallenge | null {
  if (task === undefined || !Object.prototype.hasOwnProperty.call(task.result, "captcha_challenge"))
    return null;
  const raw = task.result.captcha_challenge;
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return null;
  const value = raw as Record<string, unknown>;
  if (
    typeof value.challenge_id !== "string" ||
    typeof value.host !== "string" ||
    typeof value.image_data !== "string" ||
    typeof value.expires_at !== "string"
  )
    return null;
  if (
    value.interaction_kind !== "image_captcha_ocr" ||
    value.credential === null ||
    typeof value.credential !== "object" ||
    Array.isArray(value.credential)
  )
    return null;
  const credential = value.credential as Record<string, unknown>;
  if (typeof credential.entry !== "string") return null;
  return {
    challenge_id: value.challenge_id,
    host: value.host,
    image_data: value.image_data,
    expires_at: value.expires_at,
    credential: { entry: credential.entry },
    interaction_kind: "image_captcha_ocr",
  };
}
