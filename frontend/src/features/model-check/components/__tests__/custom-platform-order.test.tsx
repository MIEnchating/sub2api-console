import { fireEvent, screen } from "@testing-library/react";
import { useForm } from "react-hook-form";
import { expect, it } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import type { CustomAnimationForm } from "../../lib/animation-schema";
import { CustomAnimationFields } from "../custom-animation-fields";

function Form() {
  const form = useForm<CustomAnimationForm>({
    defaultValues: {
      platform: "openai",
      base_url: "",
      api_key: "",
      model: "",
      timeout_seconds: 30,
    },
  });
  return <CustomAnimationFields form={form} disabled={false} />;
}

it("平台字典逆序时自定义检测仅排序已支持协议且保留选中值", async () => {
  renderWithDictionaries(<Form />, {
    platform: [{ value: "gemini" }, { value: "anthropic" }, { value: "openai" }],
  });
  const control = screen.getByRole("combobox", { name: "接口类型" });
  fireEvent.click(control);
  const options = await screen.findAllByRole("option");
  expect(options.map((option) => option.textContent)).toEqual(["Anthropic", "OpenAI"]);
  expect(options[1]).toHaveAttribute("aria-selected", "true");
});
