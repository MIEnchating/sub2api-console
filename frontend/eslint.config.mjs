import eslint from "@eslint/js";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist", "node_modules"] },
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
  {
    rules: {
      "no-nested-ternary": "error",
      "no-restricted-globals": [
        "error",
        { name: "alert", message: "请使用项目 Dialog 或 toast 组件。" },
        { name: "confirm", message: "请使用 ConfirmActionDialog。" },
        { name: "prompt", message: "请使用项目 Dialog 表单组件。" },
      ],
      "no-restricted-properties": [
        "error",
        { object: "window", property: "alert", message: "请使用项目 Dialog 或 toast 组件。" },
        { object: "window", property: "confirm", message: "请使用 ConfirmActionDialog。" },
        { object: "window", property: "prompt", message: "请使用项目 Dialog 表单组件。" },
        { object: "globalThis", property: "alert", message: "请使用项目 Dialog 或 toast 组件。" },
        { object: "globalThis", property: "confirm", message: "请使用 ConfirmActionDialog。" },
        { object: "globalThis", property: "prompt", message: "请使用项目 Dialog 表单组件。" },
      ],
    },
  },
);
