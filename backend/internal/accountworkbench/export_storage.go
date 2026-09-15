package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const exportLifetime = 24 * time.Hour
const maxExportBytes = 64 << 20

type storedExport struct {
	Metadata ExportMetadata `json:"metadata"`
	Owner    string         `json:"owner"`
	Target   string         `json:"target"`
	timer    *time.Timer
}

type exportState struct {
	mu        sync.Mutex
	directory *os.File
	closed    bool
	previews  map[string]*preparedExport
	artifacts map[string]*storedExport
	orphans   map[string]*time.Timer
}

func openExportState(directory string) (*exportState, error) {
	file, err := openPrivateExportDirectory(directory)
	if err != nil {
		return nil, err
	}
	state := &exportState{directory: file, previews: make(map[string]*preparedExport), artifacts: make(map[string]*storedExport), orphans: make(map[string]*time.Timer)}
	if err := state.load(); err != nil {
		_ = state.close()
		return nil, err
	}
	return state, nil
}

// Open every component relative to the preceding descriptor. O_NOFOLLOW
// rejects symlinks and retains confinement even if a path is renamed later.
func openPrivateExportDirectory(directory string) (*os.File, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || directory == "/" {
		return nil, ErrExportUnavailable
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(directory, "/"), "/") {
		next, nextErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(nextErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return nil, mkdirErr
			}
			next, nextErr = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		_ = unix.Close(fd)
		if nextErr != nil {
			return nil, nextErr
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Mode&0777 != 0700 || stat.Uid != uint32(os.Geteuid()) {
		_ = unix.Close(fd)
		return nil, ErrExportUnavailable
	}
	return os.NewFile(uintptr(fd), directory), nil
}

func (s *exportState) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.directory.ReadDir(-1)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".meta.json")
		if !ok || !validExportID(id) {
			continue
		}
		file, err := s.openExisting(entry.Name())
		if err != nil {
			return err
		}
		var artifact storedExport
		decoder := json.NewDecoder(io.LimitReader(file, 4096))
		decoder.DisallowUnknownFields()
		decodeErr := decoder.Decode(&artifact)
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		_ = file.Close()
		created, createdErr := time.Parse(time.RFC3339Nano, artifact.Metadata.CreatedAt)
		expires, expiresErr := time.Parse(time.RFC3339Nano, artifact.Metadata.ExpiresAt)
		if decodeErr != nil || trailingErr != io.EOF || createdErr != nil || expiresErr != nil || artifact.Metadata.ID != id ||
			(artifact.Metadata.Kind != ExportAccounts && artifact.Metadata.Kind != ExportLoginProfiles) ||
			artifact.Metadata.Count < 1 || artifact.Metadata.Count > 500 || !validExportHash(artifact.Owner) || !validExportHash(artifact.Target) ||
			!expires.After(created) || expires.Sub(created) > exportLifetime || created.After(now.Add(time.Minute)) {
			return ErrExportUnavailable
		}
		if !now.Before(expires) {
			if err := s.unlinkArtifact(id); err != nil {
				return err
			}
			continue
		}
		payload, err := s.openExisting(id + ".json")
		if err != nil {
			return err
		}
		info, statErr := payload.Stat()
		_ = payload.Close()
		if statErr != nil || info.Size() < 1 || info.Size() > maxExportBytes {
			return ErrExportUnavailable
		}
		s.artifacts[id] = &artifact
		artifact.timer = time.AfterFunc(time.Until(expires), func() { s.expire(id) })
	}
	// Remove orphan payloads left by an interrupted write once their maximum
	// retention has elapsed. No arbitrary directory entries are touched.
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !validExportID(id) || s.artifacts[id] != nil {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !now.Before(info.ModTime().Add(exportLifetime)) {
			if err := s.unlink(entry.Name()); err != nil {
				return err
			}
		} else {
			s.orphans[id] = time.AfterFunc(time.Until(info.ModTime().Add(exportLifetime)), func() { s.expireOrphan(id) })
		}
	}
	return nil
}

func validExportID(id string) bool {
	return len(id) == 32 && strings.Trim(id, "0123456789abcdef") == ""
}
func validExportHash(hash string) bool {
	return len(hash) == 64 && strings.Trim(hash, "0123456789abcdef") == ""
}

func (s *exportState) openExisting(name string) (*os.File, error) {
	fd, err := unix.Openat(int(s.directory.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0777 != 0600 || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		_ = unix.Close(fd)
		return nil, ErrExportUnavailable
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (s *exportState) create(name string, value any) error {
	fd, err := unix.Openat(int(s.directory.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), name)
	writeErr := json.NewEncoder(&exportLimitedWriter{writer: file, remaining: maxExportBytes}).Encode(value)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = s.unlink(name)
		return writeErr
	}
	if closeErr != nil {
		_ = s.unlink(name)
	}
	return closeErr
}

type exportLimitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *exportLimitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		return 0, errors.New("导出文件超过 64 MiB，请分批导出")
	}
	written, err := w.writer.Write(data)
	w.remaining -= written
	return written, err
}

func (s *exportState) write(ctx context.Context, owner, target string, accounts []map[string]any) (ExportMetadata, error) {
	return s.writeData(ctx, owner, target, ExportAccounts, len(accounts), func(created string) (json.RawMessage, error) {
		return json.Marshal(sub2APIExport{Type: "sub2api-data", Version: 1, ExportedAt: created, Proxies: []map[string]any{}, Accounts: accounts})
	})
}

func (s *exportState) writeData(ctx context.Context, owner, target string, kind ExportKind, count int, build func(string) (json.RawMessage, error)) (ExportMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || count < 1 || count > 500 || (kind != ExportAccounts && kind != ExportLoginProfiles) {
		return ExportMetadata{}, ErrExportUnavailable
	}
	if err := ctx.Err(); err != nil {
		return ExportMetadata{}, err
	}
	id, err := randomID()
	if err != nil {
		return ExportMetadata{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(exportLifetime)
	metadata := ExportMetadata{ID: id, Kind: kind, Count: count, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: expires.Format(time.RFC3339Nano)}
	artifact := &storedExport{Metadata: metadata, Owner: owner, Target: target}
	data, err := build(metadata.CreatedAt)
	if err != nil || len(data) > maxExportBytes || !json.Valid(data) {
		return ExportMetadata{}, ErrExportUnavailable
	}
	if err := s.create(id+".json", data); err != nil {
		return ExportMetadata{}, ErrExportUnavailable
	}
	if err := s.create(id+".meta.json", artifact); err != nil {
		_ = s.unlink(id + ".json")
		return ExportMetadata{}, ErrExportUnavailable
	}
	if err := ctx.Err(); err != nil {
		_ = s.unlinkArtifact(id)
		return ExportMetadata{}, err
	}
	if err := s.directory.Sync(); err != nil {
		_ = s.unlinkArtifact(id)
		return ExportMetadata{}, ErrExportUnavailable
	}
	s.artifacts[id] = artifact
	artifact.timer = time.AfterFunc(time.Until(expires), func() { s.expire(id) })
	return metadata, nil
}

func (s *exportState) addPreview(prepared *preparedExport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrExportUnavailable
	}
	for id, previous := range s.previews {
		if previous.owner == prepared.owner || !time.Now().Before(previous.expires) {
			previous.timer.Stop()
			delete(s.previews, id)
		}
	}
	if len(s.previews) >= 20 {
		return errors.New("导出预览任务已满，请稍后重试")
	}
	s.previews[prepared.view.ID] = prepared
	prepared.timer = time.AfterFunc(time.Until(prepared.expires), func() { s.deletePreview(prepared.owner, prepared.view.ID) })
	return nil
}

func (s *exportState) deletePreview(owner, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prepared := s.previews[id]; prepared != nil && prepared.owner == owner {
		prepared.timer.Stop()
		delete(s.previews, id)
	}
}

func (s *exportState) takePreview(owner, id string, kind ExportKind) (*preparedExport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prepared := s.previews[id]
	if s.closed || prepared == nil || prepared.owner != owner || prepared.kind != kind || !time.Now().Before(prepared.expires) {
		return nil, ErrExportPreview
	}
	prepared.timer.Stop()
	delete(s.previews, id)
	return prepared, nil
}

func (s *exportState) list(owner, target string) ([]ExportMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrExportUnavailable
	}
	result := []ExportMetadata{}
	for id, artifact := range s.artifacts {
		expires, _ := time.Parse(time.RFC3339Nano, artifact.Metadata.ExpiresAt)
		if !time.Now().Before(expires) {
			if err := s.unlinkArtifact(id); err != nil {
				return nil, ErrExportUnavailable
			}
			artifact.timer.Stop()
			delete(s.artifacts, id)
			continue
		}
		if artifact.Owner == owner && artifact.Target == target {
			result = append(result, artifact.Metadata)
		}
	}
	slices.SortFunc(result, func(a, b ExportMetadata) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	return result, nil
}

func (s *exportState) remove(owner, target, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact := s.artifacts[id]
	if s.closed || artifact == nil || artifact.Owner != owner || artifact.Target != target {
		return ErrExportPreview
	}
	if err := s.unlinkArtifact(id); err != nil {
		return ErrExportUnavailable
	}
	artifact.timer.Stop()
	delete(s.artifacts, id)
	return nil
}

func (s *exportState) unlink(name string) error {
	err := unix.Unlinkat(int(s.directory.Fd()), name, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
}

func (s *exportState) unlinkArtifact(id string) error {
	if err := s.unlink(id + ".json"); err != nil {
		return err
	}
	if err := s.unlink(id + ".meta.json"); err != nil {
		return err
	}
	return s.directory.Sync()
}

func (s *exportState) expire(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact := s.artifacts[id]
	if s.closed || artifact == nil {
		return
	}
	if err := s.unlinkArtifact(id); err != nil {
		artifact.timer = time.AfterFunc(time.Minute, func() { s.expire(id) })
		return
	}
	delete(s.artifacts, id)
}

func (s *exportState) expireOrphan(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && s.artifacts[id] == nil {
		_ = s.unlink(id + ".json")
	}
	delete(s.orphans, id)
}

func (s *exportState) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for _, prepared := range s.previews {
		prepared.timer.Stop()
	}
	for _, artifact := range s.artifacts {
		artifact.timer.Stop()
	}
	for _, timer := range s.orphans {
		timer.Stop()
	}
	s.previews = nil
	s.artifacts = nil
	s.orphans = nil
	return s.directory.Close()
}
