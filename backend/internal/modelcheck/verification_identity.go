package modelcheck

func verificationAuthenticity(source string, successful int, modelStatus string) VerificationDimension {
	if successful == 0 {
		return VerificationDimension{Status: "inconclusive", Summary: "未收到有效模型回答，身份检测未完成。请先处理逐项请求错误再重新检测，不能据此判定模型真假。"}
	}
	if source != "third_party" {
		return VerificationDimension{Status: "official_source", Summary: "本次检测已从固定官方端点收到有效回答，官方请求来源已确认；仅覆盖本次检测路径，不证明生产路由或具体模型权重。返回型号差异仍需单独核对。"}
	}
	if modelStatus == "warning" {
		return VerificationDimension{Status: "evidence_mismatch", Summary: "返回型号与请求声明不同或同批返回多个型号，请核对型号映射和路由配置；别名差异不等于模型被替换。"}
	}
	return VerificationDimension{Status: "observed", Summary: "接口检查已完成，返回型号与协议证据见明细；能力与指纹采样独立展示，不据此认证模型身份。"}
}
