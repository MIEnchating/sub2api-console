import { describe, expect, it } from "vitest";
import { challengeFailureMessage } from "../constants";

describe("Cloudflare 验证失败指引", () => {
  it("站点域名未授权时要求上游检查配置", () => {
    expect(challengeFailureMessage("110200")).toContain("上游管理员检查站点密钥和域名");
  });
  it("验证超时提示检查时钟和负载后重试", () => {
    expect(challengeFailureMessage("110600")).toContain("服务器时间与负载");
  });
  it("验证资源加载失败时指出需检查的验证域名", () => {
    expect(challengeFailureMessage("200500")).toContain("challenges.cloudflare.com");
  });
  it("通用挑战失败保留错误码并提示上游检查出口", () => {
    expect(challengeFailureMessage("600010")).toContain("600010");
    expect(challengeFailureMessage("600010")).toContain("上游管理员检查服务器出口");
  });
});
