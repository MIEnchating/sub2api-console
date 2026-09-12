import type { ReactElement } from "react";
import { useState } from "react";
import type { BrowserLoginInput, BrowserLoginSession } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const browserKeys = new Set([
  "Enter",
  "Backspace",
  "Delete",
  "ArrowLeft",
  "ArrowRight",
  "ArrowUp",
  "ArrowDown",
  "Home",
  "End",
]);

export function BrowserSurface(props: {
  session: BrowserLoginSession;
  disabled: boolean;
  onInput: (value: BrowserLoginInput) => Promise<unknown>;
}): ReactElement {
  const [text, setText] = useState("");
  const send = (value: BrowserLoginInput): void => {
    void props.onInput(value).catch(() => undefined);
  };
  return (
    <div className="grid min-w-0 gap-3">
      <p className="text-muted-foreground text-sm" id="browser-login-controls">
        点击画面中的输入框后可直接键入或粘贴。Tab
        键切换控制台控件；用下方按钮切换上游输入框。中文可用文字输入框发送。
      </p>
      <div className="overflow-x-auto rounded-md border">
        <div
          role="button"
          tabIndex={props.disabled ? -1 : 0}
          aria-label="上游登录页面"
          aria-describedby="browser-login-controls"
          aria-disabled={props.disabled}
          className="relative w-full min-w-160 cursor-crosshair focus-visible:ring-2 focus-visible:ring-ring"
          onClick={(event) => {
            if (props.disabled) return;
            event.currentTarget.focus();
            const rect = event.currentTarget.getBoundingClientRect();
            send({
              kind: "click",
              x: Math.min(
                props.session.width - 1,
                Math.max(0, ((event.clientX - rect.left) * props.session.width) / rect.width),
              ),
              y: Math.min(
                props.session.height - 1,
                Math.max(0, ((event.clientY - rect.top) * props.session.height) / rect.height),
              ),
            });
          }}
          onKeyDown={(event) => {
            if (
              props.disabled ||
              event.nativeEvent.isComposing ||
              event.ctrlKey ||
              event.metaKey ||
              event.altKey
            )
              return;
            if (browserKeys.has(event.key)) {
              event.preventDefault();
              event.stopPropagation();
              send({ kind: "key", key: event.key, shift: event.shiftKey });
            } else if (event.key.length === 1) {
              event.preventDefault();
              event.stopPropagation();
              send({ kind: "text", text: event.key });
            }
          }}
          onPaste={(event) => {
            if (props.disabled) return;
            event.preventDefault();
            const value = event.clipboardData.getData("text");
            if (value) send({ kind: "text", text: value });
          }}
        >
          <img
            src={props.session.image}
            alt="服务器浏览器中的上游登录与验证页面"
            width={props.session.width}
            height={props.session.height}
            className="block h-auto w-full select-none"
            draggable={false}
          />
        </div>
      </div>
      <div role="group" aria-label="上游页面操作" className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled}
          onClick={() => send({ kind: "key", key: "Tab", shift: true })}
        >
          上一个输入框
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled}
          onClick={() => send({ kind: "key", key: "Tab" })}
        >
          下一个输入框
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled}
          onClick={() => send({ kind: "key", key: "Enter" })}
        >
          回车
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled}
          onClick={() => send({ kind: "scroll", delta: -500 })}
        >
          向上滚动
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled}
          onClick={() => send({ kind: "scroll", delta: 500 })}
        >
          向下滚动
        </Button>
      </div>
      <div className="flex gap-2">
        <Input
          type="password"
          aria-label="发送到上游当前输入框的文字"
          autoComplete="off"
          value={text}
          maxLength={4096}
          disabled={props.disabled}
          onChange={(event) => setText(event.target.value)}
          placeholder="输入文字后发送到上游当前输入框"
        />
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled || !text}
          onClick={() => {
            const value = text;
            setText("");
            send({ kind: "text", text: value });
          }}
        >
          发送文字
        </Button>
      </div>
    </div>
  );
}
