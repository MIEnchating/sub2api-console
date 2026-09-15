package accountworkbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"golang.org/x/sys/unix"
)

func cleanupFileRead(directory *os.File, name string, limit int64) ([]byte, error) {
	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0777 != 0600 || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		return nil, ErrExportUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrExportUnavailable
	}
	return raw, nil
}

func cleanupDigest(raw ...[]byte) string {
	hash := sha256.New()
	for _, value := range raw {
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cleanupFileItem(id, kind string, count int, matches []string, all bool) CleanupItem {
	sort.Strings(matches)
	item := CleanupItem{ID: id, Kind: kind, Count: count, AccountIDs: matches, Action: "retain", Reason: "包含未选账号或无法完整确认稳定身份"}
	if all {
		item.Action, item.Reason = "delete", "全部内容均属于本次选定账号"
	}
	return item
}

func cleanupProfileIdentity(profiles []configstore.WorkbenchLoginProfile, accountID, userID, workspace, email string) string {
	for _, profile := range profiles {
		if accountID != "" && profile.AccountID != accountID {
			continue
		}
		if userID == "" || profile.UserID != userID {
			continue
		}
		if workspace != "" && profile.WorkspaceID != workspace {
			continue
		}
		if email != "" && !strings.EqualFold(profile.Email, email) {
			continue
		}
		return profile.AccountID
	}
	return ""
}

func (state *exportState) cleanupExports(owner, target string, profiles []configstore.WorkbenchLoginProfile) ([]CleanupItem, map[string]string, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, nil, ErrExportUnavailable
	}
	items := []CleanupItem{}
	snapshots := map[string]string{}
	for id, artifact := range state.artifacts {
		if artifact.Owner != owner || artifact.Target != target {
			continue
		}
		raw, err := cleanupFileRead(state.directory, id+".json", maxExportBytes)
		if err != nil {
			return nil, nil, ErrExportUnavailable
		}
		matches := []string{}
		all := true
		kind := "account_export"
		count := 0
		if artifact.Metadata.Kind == ExportLoginProfiles {
			kind = "profile_export"
			var data privateProfileExport
			if json.Unmarshal(raw, &data) != nil || data.Type != "account-workbench-login-profiles" || data.Version != 1 {
				return nil, nil, ErrExportUnavailable
			}
			count = len(data.Profiles)
			for _, profile := range data.Profiles {
				match := ""
				if profile.WorkspaceID != "" {
					match = cleanupProfileIdentity(profiles, profile.AccountID, profile.UserID, profile.WorkspaceID, profile.Email)
				}
				if match == "" {
					all = false
				} else {
					matches = append(matches, match)
				}
			}
		} else if artifact.Metadata.Kind == ExportAccounts {
			var data struct {
				Type     string            `json:"type"`
				Version  int               `json:"version"`
				Accounts []json.RawMessage `json:"accounts"`
			}
			if json.Unmarshal(raw, &data) != nil || data.Type != "sub2api-data" || data.Version != 1 {
				return nil, nil, ErrExportUnavailable
			}
			count = len(data.Accounts)
			for _, account := range data.Accounts {
				parsed, failures := Parse(string(account))
				match := ""
				if len(failures) == 0 && len(parsed) == 1 {
					identity := inputIdentity(parsed[0].Credentials)
					if identity.workspace != "" {
						match = cleanupProfileIdentity(profiles, "", identity.user, identity.workspace, parsed[0].Email)
					}
				}
				if match == "" {
					all = false
				} else {
					matches = append(matches, match)
				}
			}
		} else {
			return nil, nil, ErrExportUnavailable
		}
		if count != artifact.Metadata.Count || count == 0 {
			return nil, nil, ErrExportUnavailable
		}
		if len(matches) == 0 {
			continue
		}
		item := cleanupFileItem(id, kind, count, matches, all)
		items = append(items, item)
		if all {
			snapshots[id] = cleanupDigest(raw)
		}
	}
	return items, snapshots, nil
}

func (state *exportState) cleanupRemoveExport(owner, target, id, digest string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	artifact := state.artifacts[id]
	if state.closed || artifact == nil || artifact.Owner != owner || artifact.Target != target {
		return ErrExportPreview
	}
	raw, err := cleanupFileRead(state.directory, id+".json", maxExportBytes)
	if err != nil || cleanupDigest(raw) != digest {
		return ErrExportChanged
	}
	if err := state.unlinkArtifact(id); err != nil {
		return ErrExportUnavailable
	}
	artifact.timer.Stop()
	delete(state.artifacts, id)
	return nil
}

func cleanupSecurityRead(storage *securityStorage, id string) (privateSecurityArtifact, string, error) {
	raw, err := cleanupFileRead(storage.directory, id+".json", 64<<10)
	if err != nil {
		return privateSecurityArtifact{}, "", err
	}
	var artifact privateSecurityArtifact
	if json.Unmarshal(raw, &artifact) != nil || artifact.Version != 1 {
		return artifact, "", ErrSecurityStorage
	}
	result, err := cleanupFileRead(storage.directory, id+".result.json", 64<<10)
	if errors.Is(err, unix.ENOENT) {
		result = nil
	} else if err != nil {
		return artifact, "", err
	}
	return artifact, cleanupDigest(raw, result), nil
}

func (storage *securityStorage) cleanupSecurity(target string, profiles []configstore.WorkbenchLoginProfile) ([]CleanupItem, map[string]string, error) {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if storage.closed {
		return nil, nil, ErrSecurityStorage
	}
	fd, err := unix.Openat(int(storage.directory.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, ErrSecurityStorage
	}
	directory := os.NewFile(uintptr(fd), "security-results")
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, nil, ErrSecurityStorage
	}
	items := []CleanupItem{}
	snapshots := map[string]string{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !validExportID(id) {
			continue
		}
		artifact, digest, err := cleanupSecurityRead(storage, id)
		if err != nil {
			return nil, nil, ErrSecurityStorage
		}
		if artifact.SourceOAuthID != "" || artifact.SourceOAuthBatchID != "" || artifact.SourceCheckpointID != "" || artifact.Scope == ScopeLocalExport {
			continue
		}
		match := cleanupProfileIdentity(profiles, artifact.AccountID, artifact.UserID, "", artifact.Email)
		if match == "" {
			continue
		}
		owned := artifact.TargetFingerprint == target
		items = append(items, cleanupFileItem(id, "security_result", 1, []string{match}, owned))
		if owned {
			snapshots[id] = digest
		}
	}
	return items, snapshots, nil
}

func (storage *securityStorage) cleanupRemoveSecurity(id, digest string) error {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if storage.closed {
		return ErrSecurityStorage
	}
	_, current, err := cleanupSecurityRead(storage, id)
	if err != nil || current != digest {
		return ErrSecurityStorage
	}
	for _, name := range []string{id + ".json", id + ".result.json"} {
		if err := unix.Unlinkat(int(storage.directory.Fd()), name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return ErrSecurityStorage
		}
	}
	return storage.directory.Sync()
}
