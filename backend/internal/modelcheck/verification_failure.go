package modelcheck

// Preserve the configured source without claiming a successful request.
func failedVerification(account selectedAccount, input targetRequest, err error) map[string]any {
	source := "third_party"
	if account.AccountType == "oauth" {
		source = "official_oauth"
	} else if officialVerificationEndpoint(account.BaseURL, account.Platform) {
		source = "official_endpoint"
	}
	return map[string]any{
		"version": verificationVersion, "checker": "verification", "verdict": "ERROR",
		"account_id": account.ID, "account_name": account.Name, "claimed_model": input.Model,
		"source": source, "error": err.Error(), "total": verificationProbeCount(input.Rounds, input.Fingerprint),
		"capability_total": input.Rounds * 7, "passed": 0,
		"capability":   VerificationDimension{Status: "inconclusive", Summary: "请求未能开始，请先处理账号或凭据错误。"},
		"authenticity": VerificationDimension{Status: "inconclusive", Summary: "尚未发出有效请求，不能确认来源或模型身份。"},
		"fingerprint":  verificationFingerprintReport(nil, input.Fingerprint),
		"requests":     map[string]int{"successful": 0, "total": 0, "planned": verificationProbeCount(input.Rounds, input.Fingerprint)},
	}
}
