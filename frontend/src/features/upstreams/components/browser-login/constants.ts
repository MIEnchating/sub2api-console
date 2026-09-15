export function challengeFailureMessage(code: string): string {
  const prefix = `Cloudflare 验证失败（${code}）：`;
  if (["110100", "110110", "110200", "400020", "400070"].includes(code)) {
    return `${prefix}上游验证码配置无效，请联系上游管理员检查站点密钥和域名。`;
  }
  if (["110600", "110620", "200100"].includes(code)) {
    return `${prefix}验证超时或时钟异常，请检查服务器时间与负载后刷新验证页面。`;
  }
  if (code === "200500") {
    return `${prefix}验证资源加载失败，请检查服务器到 challenges.cloudflare.com 的网络后重试。`;
  }
  return `${prefix}当前服务器浏览器未通过人机验证。可刷新后重试；持续失败请联系上游管理员检查服务器出口或申请可用于接口鉴权的凭据。`;
}
