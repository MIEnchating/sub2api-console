package accountworkbench

import (
	"context"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) previewLocalRegeneration(ctx context.Context, owner string, input RegenerationInput) (RegenerationPreview, error) {
	if err := ctx.Err(); err != nil {
		return RegenerationPreview{}, err
	}
	state, err := s.exportStorage()
	if err != nil {
		return RegenerationPreview{}, err
	}
	source, err := s.loadLocalRegenerationSource(ctx, state, exportHash(owner), input)
	if err != nil {
		return RegenerationPreview{}, err
	}
	view := RegenerationPreview{Scope: ScopeLocalExport, ArtifactID: input.ArtifactID, SourceTaskID: input.SourceTaskID, Items: make([]RegenerationItem, 0, len(source.accounts))}
	for _, account := range source.accounts {
		view.Items = append(view.Items, account.view)
	}
	expires := time.Now().Add(10 * time.Minute)
	if !source.expires.IsZero() && source.expires.Before(expires) {
		expires = source.expires
	}
	view.ID, err = randomID()
	if err != nil {
		return RegenerationPreview{}, err
	}
	view.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	stored := view
	stored.Items = append([]RegenerationItem(nil), view.Items...)
	prepared := &preparedExport{owner: exportHash(owner), view: ExportPreview{ID: view.ID}, expires: expires, kind: exportRegeneration, regeneration: &preparedRegeneration{input: input, view: stored, sourceRevision: source.revision}}
	if err := state.addPreview(prepared); err != nil {
		return RegenerationPreview{}, err
	}
	return view, nil
}

func (s *Service) loadLocalRegenerationSource(ctx context.Context, state *exportState, owner string, input RegenerationInput) (regenerationSource, error) {
	if input.Scope != ScopeLocalExport || len(input.AccountIDs) != 0 {
		return regenerationSource{}, errRegenerationSource
	}
	if err := ctx.Err(); err != nil {
		return regenerationSource{}, err
	}
	if input.ArtifactID == "" {
		source, err := s.regenerationTaskSource(ctx, state, owner, configstore.TargetSettings{}, input.SourceTaskID, ScopeLocalExport)
		if err != nil {
			return source, err
		}
		return selectRegenerationSource(source, input)
	}
	accounts, metadata, err := state.readAccountArtifact(owner, workbenchScopeFingerprint(ScopeLocalExport, configstore.TargetSettings{}), input.ArtifactID)
	if err != nil {
		return regenerationSource{}, err
	}
	revision, err := exportRevision(metadata)
	if err != nil {
		return regenerationSource{}, errRegenerationSource
	}
	expires, err := time.Parse(time.RFC3339Nano, metadata.ExpiresAt)
	if err != nil {
		return regenerationSource{}, errRegenerationSource
	}
	source := regenerationSource{revision: revision, expires: expires}
	for index, account := range accounts {
		snapshot, err := regenerationSnapshot(index, "", account, configstore.TargetSettings{})
		if err != nil {
			return source, err
		}
		source.accounts = append(source.accounts, snapshot)
	}
	return selectRegenerationSource(source, input)
}
