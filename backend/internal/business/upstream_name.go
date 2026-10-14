package business

import "strings"

func upstreamNameMode(value UpstreamConfigurationWrite) string {
	if value.NameMode != nil {
		return *value.NameMode
	}
	if value.Name != nil {
		return "manual"
	}
	return "auto"
}

func upstreamMetadataNameMode(metadata map[string]any) string {
	if stringValue(metadata["name_mode"]) == "manual" {
		return "manual"
	}
	return "auto"
}

func applyObservedUpstreamName(metadata map[string]any, name *string) {
	if name == nil || strings.TrimSpace(*name) == "" {
		return
	}
	metadata["observed_site_name"] = strings.TrimSpace(*name)
	if upstreamMetadataNameMode(metadata) != "manual" {
		metadata["site_name"] = strings.TrimSpace(*name)
	}
}
