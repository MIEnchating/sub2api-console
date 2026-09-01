import { describe, expect, it } from "vitest";

import { alertPolicyFormSchema, defaultAlertPolicyForm } from "../alert-policy-schema";

describe("alertPolicyFormSchema", () => {
  it("accepts the complete default strategy", () => {
    expect(alertPolicyFormSchema.safeParse(defaultAlertPolicyForm).success).toBe(true);
  });

  it("rejects invalid thresholds and reminder intervals", () => {
    expect(
      alertPolicyFormSchema.safeParse({
        ...defaultAlertPolicyForm,
        balance_thresholds: [{ value: "-1" }],
      }).success,
    ).toBe(false);
    expect(
      alertPolicyFormSchema.safeParse({ ...defaultAlertPolicyForm, probe_failure_streak: 0 })
        .success,
    ).toBe(false);
    expect(
      alertPolicyFormSchema.safeParse({ ...defaultAlertPolicyForm, probe_recovery_streak: 0 })
        .success,
    ).toBe(false);
    expect(
      alertPolicyFormSchema.safeParse({ ...defaultAlertPolicyForm, repeat_interval_minutes: 10081 })
        .success,
    ).toBe(false);
    expect(
      alertPolicyFormSchema.safeParse({
        ...defaultAlertPolicyForm,
        state_change_cooldown_minutes: 10081,
      }).success,
    ).toBe(false);
    expect(
      alertPolicyFormSchema.safeParse({ ...defaultAlertPolicyForm, merge_threshold: 1 }).success,
    ).toBe(false);
  });
});
