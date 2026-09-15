import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import { OnboardingGroupBindingSelect } from "../onboarding-group-binding-select";

it("字典逆序时开户仅按配置排列兼容平台的本地分组", async () => {
  renderWithDictionaries(
    <OnboardingGroupBindingSelect
      upstreamGroupName="远端"
      upstreamPlatform="openai"
      groups={[
        { id: "1", name: "A", platform: "openai" },
        { id: "2", name: "B", platform: "openai" },
        { id: "3", name: "C", platform: "anthropic" },
      ]}
      value={[]}
      disabled={false}
      disabledReason={null}
      onValueChange={vi.fn()}
    />,
    { group: [{ value: "3" }, { value: "2" }, { value: "1" }] },
  );
  fireEvent.click(screen.getByRole("combobox", { name: "远端 本地分组" }));
  const options = await screen.findAllByRole("option");
  expect(options).toHaveLength(2);
  expect(options[0]).toHaveTextContent("B");
  expect(options[1]).toHaveTextContent("A");
});
