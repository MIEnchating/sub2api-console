package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type ProfileExportSelection struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type ProfileExportInput struct {
	Items []ProfileExportSelection `json:"items"`
}

type ProfileExportPreview struct {
	ID        string             `json:"id"`
	Kind      ExportKind         `json:"kind"`
	Target    string             `json:"target"`
	ExpiresAt string             `json:"expires_at"`
	Items     []LoginProfileView `json:"items"`
}

type privateProfileExport struct {
	Type       string                   `json:"type"`
	Version    int                      `json:"version"`
	ExportedAt string                   `json:"exported_at"`
	Profiles   []privateExportedProfile `json:"profiles"`
}

type privateExportedProfile struct {
	LoginProfileView
	Login json.RawMessage `json:"login"`
}

// PreviewProfileExport reads summaries only; the selected private login payloads
// are reread under the mutation lease after explicit confirmation.
func (s *Service) PreviewProfileExport(ctx context.Context, owner string, input ProfileExportInput) (ProfileExportPreview, error) {
	if owner == "" {
		return ProfileExportPreview{}, ErrExportPreview
	}
	if len(input.Items) < 1 || len(input.Items) > 500 {
		return ProfileExportPreview{}, errors.New("请选择 1～500 份登录资料")
	}
	seen := make(map[string]bool, len(input.Items))
	for _, item := range input.Items {
		if item.ID == "" || len(item.ID) > 128 || strings.TrimSpace(item.ID) != item.ID || item.Revision < 1 || seen[item.ID] {
			return ProfileExportPreview{}, errors.New("登录资料选择或版本无效，请刷新后重新选择")
		}
		seen[item.ID] = true
	}
	state, err := s.exportStorage()
	if err != nil {
		return ProfileExportPreview{}, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return ProfileExportPreview{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return ProfileExportPreview{}, err
	}
	store, err := s.profileStore()
	if err != nil {
		return ProfileExportPreview{}, err
	}
	profiles, err := store.WorkbenchLoginProfiles(ctx, executionTargetFingerprint(target))
	if err != nil {
		return ProfileExportPreview{}, err
	}
	available := make(map[string]configstore.WorkbenchLoginProfile, len(profiles))
	for _, profile := range profiles {
		available[profile.ID] = profile
	}
	view := ProfileExportPreview{Kind: ExportLoginProfiles, Target: target.BaseURL, Items: make([]LoginProfileView, 0, len(input.Items))}
	for _, item := range input.Items {
		profile, found := available[item.ID]
		if !found || profile.Revision != item.Revision {
			return ProfileExportPreview{}, configstore.ErrWorkbenchLoginProfile
		}
		view.Items = append(view.Items, loginProfileView(profile))
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return ProfileExportPreview{}, err
	}
	view.ID, err = randomID()
	if err != nil {
		return ProfileExportPreview{}, err
	}
	expires := time.Now().Add(10 * time.Minute)
	view.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	stored := view
	stored.Items = append([]LoginProfileView(nil), view.Items...)
	if err := state.addPreview(&preparedExport{owner: exportHash(owner), target: target, kind: ExportLoginProfiles, view: ExportPreview{ID: view.ID}, expires: expires, profileView: &stored}); err != nil {
		return ProfileExportPreview{}, err
	}
	return view, nil
}

func (s *Service) ExportProfiles(ctx context.Context, owner, id string, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认登录资料私有导出范围")
	}
	if owner == "" {
		return taskstore.Task{}, ErrExportPreview
	}
	state, err := s.exportStorage()
	if err != nil {
		return taskstore.Task{}, err
	}
	prepared, err := state.takePreview(exportHash(owner), id, ExportLoginProfiles)
	if err != nil {
		return taskstore.Task{}, err
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, prepared.target), s.private); err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "account-workbench-profile-export", "等待生成私有登录资料文件", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		rows = []ResultItem{{Index: 0, Name: "私有登录资料文件", Status: "running", Message: "正在核对登录资料版本"}}
		defer func() {
			if runErr != nil {
				rows[0].Status, rows[0].Message = "failed", "登录资料文件未生成，请重新预览并核对资料与管理目标"
				if errors.Is(runErr, context.Canceled) {
					rows[0].Status, rows[0].Message = "cancelled", "登录资料导出已取消"
				}
			}
		}()
		if err := update(rows); err != nil {
			return rows, err
		}
		metadata, err := s.writeProfileExport(targetguard.Expect(run, prepared.target), state, prepared)
		if err != nil {
			return rows, err
		}
		rows[0].Status, rows[0].Message, rows[0].Report = "succeeded", "私有登录资料文件已生成", exportMetadataReport(metadata)
		return rows, nil
	})
}

func (s *Service) writeProfileExport(ctx context.Context, state *exportState, prepared *preparedExport) (ExportMetadata, error) {
	guarded, release, err := targetguard.Acquire(ctx, s.repository)
	if err != nil {
		return ExportMetadata{}, err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return ExportMetadata{}, err
	}
	store, err := s.profileStore()
	if err != nil {
		return ExportMetadata{}, err
	}
	data := privateProfileExport{Type: "account-workbench-login-profiles", Version: 1, Profiles: make([]privateExportedProfile, 0, len(prepared.profileView.Items))}
	totalBytes := 0
	for _, item := range prepared.profileView.Items {
		if err := guarded.Err(); err != nil {
			return ExportMetadata{}, err
		}
		profile, err := store.WorkbenchLoginProfile(guarded, executionTargetFingerprint(prepared.target), item.ID)
		if err != nil || loginProfileView(profile) != item || !json.Valid(profile.Login) {
			return ExportMetadata{}, configstore.ErrWorkbenchLoginProfile
		}
		totalBytes += len(profile.Login)
		if totalBytes > maxExportBytes {
			return ExportMetadata{}, errors.New("登录资料导出超过 64 MiB，请分批导出")
		}
		data.Profiles = append(data.Profiles, privateExportedProfile{LoginProfileView: item, Login: profile.Login})
	}
	if _, err := targetguard.Pin(guarded, s.private); err != nil {
		return ExportMetadata{}, err
	}
	return state.writeData(guarded, prepared.owner, exportTargetHash(prepared.target), ExportLoginProfiles, len(data.Profiles), func(created string) (json.RawMessage, error) { data.ExportedAt = created; return json.Marshal(data) })
}
