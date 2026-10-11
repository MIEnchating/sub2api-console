export const sessionExpiredEvent = "sub2api-console:session-expired";
export const sessionExpiredMessage = "登录已过期，请重新登录";

let sessionGeneration = 0;

export function currentSessionGeneration(): number {
  return sessionGeneration;
}

export function advanceSessionGeneration(): void {
  sessionGeneration += 1;
}

export class SessionChangedError extends Error {
  constructor() {
    super("会话已切换，已忽略上一会话的响应");
    this.name = "SessionChangedError";
  }
}

export function assertCurrentSession(generation: number): void {
  if (generation !== sessionGeneration) throw new SessionChangedError();
}

export class SessionExpiredError extends Error {
  readonly status = 401;

  constructor() {
    super(sessionExpiredMessage);
    this.name = "SessionExpiredError";
  }
}

export function signalSessionExpired(): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new Event(sessionExpiredEvent));
}

export function isSessionExpiredError(error: unknown): error is SessionExpiredError {
  return error instanceof SessionExpiredError;
}
