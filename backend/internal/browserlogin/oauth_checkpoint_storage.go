package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"golang.org/x/sys/unix"
)

const maxCheckpointBytes = 2 << 20

type oauthCheckpointDocument struct {
	Version int                    `json:"version"`
	Meta    OAuthCheckpoint        `json:"meta"`
	Options OAuthOptions           `json:"options"`
	URL     string                 `json:"url"`
	Cookies []*network.CookieParam `json:"cookies"`
	Storage oauthPageStorage       `json:"storage"`
}

type oauthPageStorage struct {
	URL     string            `json:"url"`
	Local   map[string]string `json:"local"`
	Session map[string]string `json:"session"`
}

type oauthCheckpointDirectory struct {
	dir  *os.File
	lock *os.File
}

func openOAuthCheckpointDirectory(path string) (*oauthCheckpointDirectory, error) {
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, ErrOAuthCheckpoint
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, ErrOAuthCheckpoint
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrOAuthCheckpoint
	}
	dir := os.NewFile(uintptr(fd), path)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&0777 != 0700 || stat.Uid != uint32(os.Geteuid()) {
		dir.Close()
		return nil, ErrOAuthCheckpoint
	}
	lockFD, err := unix.Openat(fd, ".lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		dir.Close()
		return nil, ErrOAuthCheckpoint
	}
	lock := os.NewFile(uintptr(lockFD), ".lock")
	if unix.Flock(lockFD, unix.LOCK_EX|unix.LOCK_NB) != nil {
		lock.Close()
		dir.Close()
		return nil, ErrOAuthCheckpoint
	}
	return &oauthCheckpointDirectory{dir: dir, lock: lock}, nil
}

func (d *oauthCheckpointDirectory) close() {
	_ = unix.Flock(int(d.lock.Fd()), unix.LOCK_UN)
	_ = d.lock.Close()
	_ = d.dir.Close()
}

func (d *oauthCheckpointDirectory) read(id string) (oauthCheckpointDocument, error) {
	var document oauthCheckpointDocument
	if len(id) != 48 || !validRecoveryToken(id) {
		return document, ErrOAuthCheckpoint
	}
	fd, err := unix.Openat(int(d.dir.Fd()), id+".json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return document, err
	}
	file := os.NewFile(uintptr(fd), id+".json")
	defer file.Close()
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0777 != 0600 || stat.Uid != uint32(os.Geteuid()) || stat.Size > maxCheckpointBytes {
		return document, ErrOAuthCheckpoint
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxCheckpointBytes+1))
	if err != nil || len(raw) > maxCheckpointBytes || json.Unmarshal(raw, &document) != nil || document.Version != 1 || document.Meta.ID != id {
		return oauthCheckpointDocument{}, ErrOAuthCheckpoint
	}
	return document, nil
}

func (d *oauthCheckpointDirectory) remove(id string) error {
	err := unix.Unlinkat(int(d.dir.Fd()), id+".json", 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return ErrOAuthCheckpoint
	}
	if d.dir.Sync() != nil {
		return ErrOAuthCheckpoint
	}
	return nil
}

func (f Chromium) saveOAuthCheckpoint(document oauthCheckpointDocument) (OAuthCheckpoint, error) {
	dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
	if err != nil {
		return OAuthCheckpoint{}, err
	}
	defer dir.close()
	if document.Meta.ID == "" {
		rawID := make([]byte, 24)
		if _, err := rand.Read(rawID); err != nil {
			return OAuthCheckpoint{}, ErrOAuthCheckpoint
		}
		document.Meta.ID = hex.EncodeToString(rawID)
	}
	if document.Meta.Revision <= 0 {
		document.Meta.Revision = 1
	}
	if len(document.Meta.ID) != 48 || !validRecoveryToken(document.Meta.ID) {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	raw, err := json.Marshal(document)
	if err != nil || len(raw) > maxCheckpointBytes {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	name := document.Meta.ID + ".json"
	fd, err := unix.Openat(int(dir.dir.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	file := os.NewFile(uintptr(fd), name)
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil || dir.dir.Sync() != nil {
		_ = dir.remove(document.Meta.ID)
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	ref := OAuthCheckpointRef{ID: document.Meta.ID, Owner: document.Meta.Owner, Lease: document.Meta.Lease}
	time.AfterFunc(time.Until(document.Meta.ExpiresAt), func() { _ = f.DeleteOAuthCheckpoint(context.Background(), ref) })
	return document.Meta, nil
}

func (f Chromium) DeleteOAuthCheckpoint(ctx context.Context, ref OAuthCheckpointRef) error {
	if ctx.Err() != nil || ref.validate() != nil {
		return ErrOAuthCheckpoint
	}
	dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
	if err != nil {
		return err
	}
	defer dir.close()
	document, err := dir.read(ref.ID)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil || document.Meta.Owner != ref.Owner || document.Meta.Lease != ref.Lease {
		return ErrOAuthCheckpoint
	}
	return dir.remove(ref.ID)
}

// A successful claim removes the private snapshot before opening Chromium.
// A lost restore response cannot make the same snapshot executable twice.
func (f Chromium) claimOAuthCheckpoint(options OAuthRestoreOptions) (oauthCheckpointDocument, error) {
	if options.validate() != nil {
		return oauthCheckpointDocument{}, ErrOAuthCheckpoint
	}
	dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
	if err != nil {
		return oauthCheckpointDocument{}, err
	}
	defer dir.close()
	document, err := dir.read(options.Checkpoint.ID)
	if err != nil {
		return oauthCheckpointDocument{}, ErrOAuthCheckpoint
	}
	if err := validateCheckpointRestore(document, options); err != nil {
		return oauthCheckpointDocument{}, err
	}
	if err := dir.remove(document.Meta.ID); err != nil {
		return oauthCheckpointDocument{}, err
	}
	return document, nil
}

func validateCheckpointRestore(document oauthCheckpointDocument, options OAuthRestoreOptions) error {
	if options.validate() != nil {
		return ErrOAuthCheckpoint
	}
	stored, requested := document.Options, options.Options
	if document.Meta.Owner != options.Checkpoint.Owner || document.Meta.Lease != options.Checkpoint.Lease || !document.Meta.ExpiresAt.After(time.Now()) || stored.Recovery == nil || !document.Meta.ExpiresAt.Equal(requested.Recovery.ExpiresAt) || stored.AuthorizationURL != requested.AuthorizationURL || stored.State != requested.State || stored.RedirectURI != requested.RedirectURI || stored.ProxyURL != requested.ProxyURL || document.URL != document.Storage.URL || checkpointPageStage(document.URL) != document.Meta.Stage || document.Meta.Stage == "" {
		return ErrOAuthCheckpoint
	}
	if stored.Recovery.AutoCheckpoint != requested.Recovery.AutoCheckpoint || stored.Recovery.CheckpointID != requested.Recovery.CheckpointID || stored.Recovery.Owner != document.Meta.Owner || stored.Recovery.Lease != document.Meta.Lease {
		return ErrOAuthCheckpoint
	}
	if stored.Recovery.AutoCheckpoint && (options.Revision <= 0 || options.Revision != document.Meta.Revision) || options.Revision > 0 && options.Revision != document.Meta.Revision {
		return ErrOAuthCheckpoint
	}
	return nil
}

func (f Chromium) ReadOAuthCheckpoint(ctx context.Context, ref OAuthCheckpointRef) (OAuthCheckpoint, error) {
	if ctx.Err() != nil || ref.validate() != nil {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
	if err != nil {
		return OAuthCheckpoint{}, err
	}
	defer dir.close()
	document, err := dir.read(ref.ID)
	if err != nil || document.Meta.Owner != ref.Owner || document.Meta.Lease != ref.Lease || !document.Meta.ExpiresAt.After(time.Now()) || document.Meta.Revision <= 0 || document.Meta.Stage == "" || checkpointPageStage(document.URL) != document.Meta.Stage {
		return OAuthCheckpoint{}, ErrOAuthCheckpoint
	}
	return document.Meta, nil
}

func (f Chromium) PruneOAuthCheckpoints() error {
	if f.CheckpointDirectory == "" {
		return nil
	}
	dir, err := openOAuthCheckpointDirectory(f.CheckpointDirectory)
	if err != nil {
		return err
	}
	defer dir.close()
	entries, err := dir.dir.ReadDir(-1)
	if err != nil {
		return ErrOAuthCheckpoint
	}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if len(id) != 48 || !validRecoveryToken(id) || entry.Name() != id+".json" {
			continue
		}
		document, err := dir.read(id)
		if err != nil || !document.Meta.ExpiresAt.After(time.Now()) {
			if err := dir.remove(id); err != nil {
				return err
			}
		}
	}
	return nil
}
