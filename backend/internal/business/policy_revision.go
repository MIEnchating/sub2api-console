package business

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var ErrPolicyRevisionConflict = errors.New("策略已被其他操作修改，请刷新策略后重新应用本次改动")

func policyRevision(document map[string]any) (string, error) {
	data, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
