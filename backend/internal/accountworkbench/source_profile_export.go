package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const exportSourceProfiles ExportKind = "source-profiles"

func (s *Service) PreviewSourceProfileExport(ctx context.Context, owner string, input SourceProfileSelectionInput) (SourceProfileExportPreview, error) {
	if input.FreshLogin || input.FailedBatchID != "" {
		return SourceProfileExportPreview{}, ErrExportPreview
	}
	profiles, err := s.selectedSourceProfiles(ctx, owner, input)
	if err != nil {
		return SourceProfileExportPreview{}, err
	}
	state, err := s.exportStorage()
	if err != nil {
		return SourceProfileExportPreview{}, err
	}
	id, err := randomID()
	if err != nil {
		return SourceProfileExportPreview{}, err
	}
	expires := time.Now().Add(10 * time.Minute)
	view := SourceProfileExportPreview{ID: id, Scope: ScopeLocalExport, Kind: ExportLoginProfiles, ExpiresAt: expires.UTC().Format(time.RFC3339Nano), Items: make([]SourceProfileView, 0, len(profiles))}
	for _, profile := range profiles {
		view.Items = append(view.Items, sourceProfileView(profile))
	}
	stored := view
	stored.Items = append([]SourceProfileView(nil), view.Items...)
	err = state.addPreview(&preparedExport{owner: exportHash(owner), view: ExportPreview{ID: id}, kind: exportSourceProfiles, expires: expires, sourceProfileView: &stored})
	return view, err
}

func (s *Service) ExportSourceProfiles(ctx context.Context, owner, id string, confirmed bool) (taskstore.Task, error) {
	if !confirmed || owner == "" {
		return taskstore.Task{}, ErrExportPreview
	}
	state, err := s.exportStorage()
	if err != nil {
		return taskstore.Task{}, err
	}
	prepared, err := state.takePreview(exportHash(owner), id, exportSourceProfiles)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "account-workbench-source-profile-export", "等待生成本地登录资料私有文件", func(run context.Context, update func([]ResultItem) error) (rows []ResultItem, runErr error) {
		rows = []ResultItem{{Index: 0, Name: "本地登录资料文件", Status: "running", Message: "正在核对本地资料版本"}}
		defer func() {
			if runErr != nil {
				rows[0].Status, rows[0].Message = "failed", "本地资料文件未生成，请刷新资料后重新预览"
				if errors.Is(runErr, context.Canceled) {
					rows[0].Status, rows[0].Message = "cancelled", "本地资料导出已取消"
				}
			}
		}()
		if err := update(rows); err != nil {
			return rows, err
		}
		metadata, err := s.writeSourceProfiles(run, state, prepared)
		if err != nil {
			return rows, err
		}
		rows[0].Status, rows[0].Message, rows[0].Report = "succeeded", "本地登录资料私有文件已生成", localExportMetadataReport(metadata)
		return rows, nil
	})
}

func (s *Service) writeSourceProfiles(ctx context.Context, state *exportState, prepared *preparedExport) (ExportMetadata, error) {
	guarded, release, err := mutationguard.Acquire(ctx, s.repository, "workbench-source-profiles/"+prepared.owner)
	if err != nil {
		return ExportMetadata{}, err
	}
	defer func() { _ = release() }()
	store, err := s.sourceProfileStore()
	if err != nil {
		return ExportMetadata{}, err
	}
	type exportedSourceProfile struct {
		SourceProfileView
		Login json.RawMessage `json:"login"`
	}
	data := struct {
		Type       string                  `json:"type"`
		Version    int                     `json:"version"`
		Scope      ExportScope             `json:"scope"`
		ExportedAt string                  `json:"exported_at"`
		Profiles   []exportedSourceProfile `json:"profiles"`
	}{Type: "account-workbench-login-profiles", Version: 1, Scope: ScopeLocalExport, Profiles: make([]exportedSourceProfile, 0, len(prepared.sourceProfileView.Items))}
	total := 0
	for _, view := range prepared.sourceProfileView.Items {
		profile, err := store.WorkbenchSourceProfile(guarded, prepared.owner, string(ScopeLocalExport), view.ID)
		if err != nil || sourceProfileView(profile) != view || !json.Valid(profile.Login) {
			return ExportMetadata{}, configstore.ErrWorkbenchSourceProfile
		}
		total += len(profile.Login)
		if total > maxExportBytes {
			return ExportMetadata{}, errors.New("登录资料超过 64 MiB，请分批导出")
		}
		data.Profiles = append(data.Profiles, exportedSourceProfile{SourceProfileView: view, Login: profile.Login})
	}
	return state.writeData(guarded, prepared.owner, workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), ExportLoginProfiles, len(data.Profiles), func(created string) (json.RawMessage, error) { data.ExportedAt = created; return json.Marshal(data) })
}
