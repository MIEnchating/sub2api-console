package accountworkbench

type indexedAccountIdentity struct {
	id       string
	identity oauthIdentity
}

type accountIdentityIndex struct {
	stable       map[[2]string][]indexedAccountIdentity
	fingerprints map[string][]indexedAccountIdentity
}

func newAccountIdentityIndex(accounts []map[string]any) accountIdentityIndex {
	index := accountIdentityIndex{stable: make(map[[2]string][]indexedAccountIdentity), fingerprints: make(map[string][]indexedAccountIdentity)}
	for _, account := range accounts {
		if inputText(account["platform"]) != "openai" || inputText(account["type"]) != "oauth" {
			continue
		}
		identity := accountIdentity(account)
		entry := indexedAccountIdentity{id: stringValue(account["id"]), identity: identity}
		if identity.workspace != "" && identity.user != "" {
			key := [2]string{identity.workspace, identity.user}
			index.stable[key] = append(index.stable[key], entry)
		}
		if identity.fingerprint != "" {
			index.fingerprints[identity.fingerprint] = append(index.fingerprints[identity.fingerprint], entry)
		}
	}
	return index
}

func (index accountIdentityIndex) candidates(identity oauthIdentity) []indexedAccountIdentity {
	if identity.workspace != "" && identity.user != "" {
		return index.stable[[2]string{identity.workspace, identity.user}]
	}
	// Partial identities still need conflict checks before the exact-token fallback.
	return index.fingerprints[identity.fingerprint]
}
