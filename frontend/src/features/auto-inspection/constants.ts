export const inspectionAuthRecoveryActions: Readonly<Record<string, string>> = {
  browser_challenge_required:
    "请先在上游网站完成验证并重新登录，再在上游管理中找到该 Host，选择「恢复鉴权」，填写新的 Token 和刷新 Token 后验证并保存。若上游绑定 IP 或 User-Agent，登录环境须与控制台请求保持一致，并在自定义 Headers 中填写对应 User-Agent；仍失败时请联系上游管理员。",
};
