import { describe, expect, it } from "vitest";

import {
  defaultVaultEntryForHost,
  vaultEntriesForHost,
  vaultEntryLabel,
} from "../vault-entry-label";

describe("vaultEntryLabel", () => {
  it("shows the credential name without exposing its internal container", () => {
    expect(vaultEntryLabel({ entry: "大写" })).toBe("大写");
    expect(vaultEntryLabel({ entry: "operator" })).toBe("operator");
  });

  it("lists all complete entries, prioritizes Host matches, and removes duplicates", () => {
    const entries = [
      {
        entry: "default",
        hosts: [],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        entry: "matched",
        hosts: ["https://example.test/"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        entry: "wrong",
        hosts: ["other.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];
    expect(vaultEntriesForHost(entries, "example.test").map(vaultEntryLabel)).toEqual([
      "matched",
      "default",
      "wrong",
    ]);
  });

  it("excludes non-email credentials when the upstream requires an email login", () => {
    const entries = [
      {
        entry: "username",
        hosts: ["example.test"],
        has_username: true,
        has_password: true,
        username_is_email: false,
        header_names: [],
      },
      {
        entry: "email",
        hosts: ["example.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];
    expect(
      vaultEntriesForHost(entries, "example.test", { requireEmail: true }).map(vaultEntryLabel),
    ).toEqual(["email"]);
  });

  it("defaults only to the credential explicitly bound to the current Host", () => {
    const entries = [
      {
        entry: "global",
        hosts: [],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        entry: "matched",
        hosts: ["https://example.test/"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];
    expect(defaultVaultEntryForHost(entries, "example.test")).toBe("matched");
    expect(defaultVaultEntryForHost(entries, "other.test")).toBe("");
  });

  it("treats the www Host and its root-domain credential binding as the same upstream", () => {
    const entries = [
      {
        entry: "xiaobaishu",
        hosts: ["xiaobaishu.org"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];
    expect(defaultVaultEntryForHost(entries, "www.xiaobaishu.org")).toBe("xiaobaishu");
  });
});
