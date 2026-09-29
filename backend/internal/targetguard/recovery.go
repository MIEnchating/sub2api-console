package targetguard

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func Fingerprint(target configstore.TargetSettings) string {
	digest := sha256.Sum256([]byte(normalizeBaseURL(target.BaseURL) + "\x00" + strings.TrimSpace(target.AdminKey)))
	return hex.EncodeToString(digest[:])
}
