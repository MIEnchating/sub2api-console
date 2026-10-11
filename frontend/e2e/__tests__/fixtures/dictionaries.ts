import type { DictionaryKind } from "../../../src/api";
import { dictionaryItems, standardDictionaryFixtures } from "../../../src/test/dictionary-fixtures";

export function dictionaryResponse(url: string, groups: { value: string; name: string }[] = []) {
  const kind = new URL(url).searchParams.get("kind") as DictionaryKind;
  const entries = kind === "group" ? groups : (standardDictionaryFixtures[kind] ?? []);
  return { items: dictionaryItems(kind, entries) };
}
