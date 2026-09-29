package onboarding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Older pending records have only a digest. Never infer missing parameters or
// discard their write markers: they can only resume with a matching request.
func readFrozenOnboardingIntent(pending *business.PendingOnboarding) (*frozenOnboardingIntent, error) {
	if pending == nil || pending.FrozenIntentJSON == "" {
		return nil, nil
	}
	digest := sha256.Sum256([]byte(pending.FrozenIntentJSON))
	if hex.EncodeToString(digest[:]) != pending.IntentHash {
		return nil, errors.New("待续开户冻结参数校验失败，已拒绝远端写入")
	}
	var intent frozenOnboardingIntent
	if err := json.Unmarshal([]byte(pending.FrozenIntentJSON), &intent); err != nil {
		return nil, errors.New("待续开户冻结参数损坏，已拒绝远端写入")
	}
	if intent.Concurrency < 1 || intent.Concurrency > 10_000_000 || intent.Priority < 1 || intent.Priority > 10_000_000 {
		return nil, errors.New("待续开户冻结参数不完整，已拒绝远端写入")
	}
	return &intent, nil
}

func useFrozenAllocation(request *Request, intent frozenOnboardingIntent) {
	if request.Concurrency == nil {
		request.Concurrency = &intent.Concurrency
		request.WaitingForCapacity = intent.WaitingForCapacity
	}
}

func (intent frozenOnboardingIntent) creationPolicy() configstore.AccountCreationPolicy {
	return configstore.AccountCreationPolicy{
		Models: intent.Models, Priority: intent.Priority, Concurrency: intent.Concurrency,
		LoadFactor: intent.LoadFactor, PoolMode: intent.PoolMode,
		PoolModeRetryCount: intent.PoolRetryCount, PoolModeRetryStatusCodes: intent.PoolRetryStatusCodes,
	}
}
