import { describe, expect, it } from "vitest";
import type { Task } from "../../api";
import { captchaChallengeFromTask } from "../captcha-challenge";

function taskWith(challenge: unknown): Task {
  return {
    id: "task-1",
    skill: "sub2api-upstream-auth",
    operation: "recover-host",
    status: "failed",
    progress: 100,
    message: "等待验证码",
    result: { captcha_challenge: challenge },
    created_at: "2026-08-24T00:00:00Z",
    updated_at: "2026-08-24T00:00:00Z",
  };
}

describe("captchaChallengeFromTask", () => {
  it("returns a complete explicit challenge without substituting fields", () => {
    const challenge = {
      challenge_id: "challenge-1234567890",
      host: "api.example.test",
      image_data: "data:image/png;base64,AA==",
      expires_at: "2026-08-24T00:05:00Z",
      credential: { entry: "primary" },
      interaction_kind: "image_captcha_ocr",
    };

    expect(captchaChallengeFromTask(taskWith(challenge))).toEqual(challenge);
  });

  it.each([
    null,
    "",
    [],
    { challenge_id: null },
    {
      challenge_id: "challenge-1234567890",
      host: "",
      image_data: "",
      expires_at: "",
      credential: null,
      interaction_kind: "image_captcha_ocr",
    },
  ])("rejects an incomplete present challenge instead of inventing defaults", (challenge) => {
    expect(captchaChallengeFromTask(taskWith(challenge))).toBeNull();
  });
});
