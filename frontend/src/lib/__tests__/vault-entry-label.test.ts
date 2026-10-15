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
        id: "default",
        entry: "default",
        hosts: [],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        id: "matched",
        entry: "matched",
        hosts: ["https://example.test/"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        id: "wrong",
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
        id: "username",
        entry: "username",
        hosts: ["example.test"],
        has_username: true,
        has_password: true,
        username_is_email: false,
        header_names: [],
      },
      {
        id: "email",
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
        id: "global",
        entry: "global",
        hosts: [],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        id: "matched",
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
        id: "root-domain-entry",
        entry: "root-domain-entry",
        hosts: ["example.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];
    expect(defaultVaultEntryForHost(entries, "www.example.test")).toBe("root-domain-entry");
  });

  it("prefers an explicit www Host binding when root-domain credentials also exist", () => {
    const entries = [
      {
        id: "root-domain-entry",
        entry: "root-domain-entry",
        hosts: ["example.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        id: "www-entry",
        entry: "www-entry",
        hosts: ["https://www.example.test/"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];

    expect(defaultVaultEntryForHost(entries, "WWW.EXAMPLE.TEST")).toBe("www-entry");
  });

  it("places exact Host credentials before a www alias in the available options", () => {
    const entries = [
      {
        id: "www-entry",
        entry: "www-entry",
        hosts: ["www.example.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
      {
        id: "root-domain-entry",
        entry: "root-domain-entry",
        hosts: ["example.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: [],
      },
    ];

    expect(vaultEntriesForHost(entries, "example.test").map(vaultEntryLabel)).toEqual([
      "root-domain-entry",
      "www-entry",
    ]);
  });
});
