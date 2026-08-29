export const sessionExpiredEvent = "sub2api-console:session-expired";
export const sessionExpiredMessage = "登录已过期，请重新登录";

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
