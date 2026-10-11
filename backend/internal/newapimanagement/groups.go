package newapimanagement

import "context"

type RemoteGroups struct {
	Groups []RemoteGroup `json:"groups"`
}

// Groups reads only the options needed for binding, without pricing catalogs.
func (s *Service) Groups(ctx context.Context, platformID string) (RemoteGroups, error) {
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return RemoteGroups{}, err
	}
	options, err := s.readOptions(ctx, *platform)
	if err != nil {
		return RemoteGroups{}, err
	}
	groups, err := decodeGroups(options["GroupRatio"])
	return RemoteGroups{Groups: groups}, err
}
