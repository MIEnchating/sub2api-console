import { Moon, Sun } from "lucide-react";
import type { ReactElement, ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import "./login.css";
import { LoginOrchestration } from "./login-orchestration";

type LoginShellProps = {
  children: ReactNode;
  theme?: "light" | "dark";
  onThemeChange?: () => void;
};

export function LoginShell(props: LoginShellProps): ReactElement {
  const themeLabel = props.theme === "dark" ? "切换亮色主题" : "切换暗色主题";
  return (
    <div className="login-shell grid min-h-svh grid-rows-[auto_1fr] bg-background text-foreground">
      <header className="h-[var(--app-header-height)] border-b border-border bg-sidebar">
        <div className="flex h-full items-center justify-between gap-4 px-3 sm:px-4">
          <div className="flex min-w-0 items-center gap-1.5">
            <img
              src="/console-mark.svg"
              width={20}
              height={20}
              alt=""
              className="size-5 shrink-0"
            />
            <span className="text-sm font-medium">Sub2API</span>
            <span aria-hidden="true" className="mx-1 h-4 border-l" />
            <span className="text-sm text-muted-foreground">控制台</span>
          </div>
          {props.onThemeChange ? (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    aria-label={themeLabel}
                    onClick={props.onThemeChange}
                  />
                }
              >
                {props.theme === "dark" ? <Sun aria-hidden="true" /> : <Moon aria-hidden="true" />}
              </TooltipTrigger>
              <TooltipContent>{themeLabel}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
      </header>
      <main className="mx-auto grid w-full max-w-6xl content-start items-center gap-6 px-6 py-6 sm:px-8 sm:py-10 lg:grid-cols-[minmax(0,1fr)_400px] lg:content-center lg:gap-12 lg:py-12 xl:max-w-7xl xl:grid-cols-[minmax(0,1fr)_425px] xl:gap-16 2xl:max-w-[1440px] 2xl:gap-20">
        <aside
          aria-label="控制台介绍"
          className="mx-auto flex w-full min-w-0 max-w-[800px] flex-col gap-4 text-center lg:gap-6 lg:text-left"
        >
          <div className="lg:pl-6">
            <h2 className="text-2xl font-semibold leading-tight lg:text-4xl 2xl:text-[40px]">
              Sub2API Console
            </h2>
            <p className="mt-2 text-sm text-muted-foreground lg:mt-3 lg:text-base">
              Sub2API 的自动化控制台
            </p>
          </div>
          <div className="mx-auto h-36 w-full sm:h-44 lg:aspect-[34/19] lg:h-auto">
            <LoginOrchestration />
          </div>
        </aside>
        <div className="mx-auto w-full max-w-[360px] border-t border-border pt-6 lg:max-w-none lg:border-t-0 lg:border-l lg:py-8 lg:pl-10 xl:pl-16">
          {props.children}
        </div>
      </main>
    </div>
  );
}
