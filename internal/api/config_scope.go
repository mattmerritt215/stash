package api

import (
	"context"

	"github.com/mitchellh/mapstructure"

	"github.com/stashapp/stash/pkg/auth"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/utils"
)

// userInterfaceKey is the reserved key inside a user's personal ui config
// under which their ConfigInterface overrides are stored.
const userInterfaceKey = "interface"

// scopeConfigResult adapts a server-wide ConfigResult for the calling user:
// it overlays their personal interface and ui settings, and strips the
// sections that only an administrator may see.
//
// This is the read half of per-user personalisation. The write half is in
// scopedConfigureUI / scopedConfigureInterface below.
func scopeConfigResult(ctx context.Context, ret *ConfigResult) *ConfigResult {
	am := authManager()
	if am == nil || !am.Enabled() {
		return ret
	}

	u := auth.FromContext(ctx)
	if u == nil {
		return ret
	}

	personal := u.UI()

	// overlay the user's own ui config on top of the server defaults
	if len(personal) > 0 {
		merged := map[string]interface{}{}
		for k, v := range ret.UI {
			merged[k] = v
		}
		utils.MergeMaps(merged, personal)
		delete(merged, userInterfaceKey)
		ret.UI = merged
	}

	// overlay the user's own interface settings
	if raw, ok := personal[userInterfaceKey]; ok {
		if overrides, ok := raw.(map[string]interface{}); ok && len(overrides) > 0 {
			if err := mapstructure.Decode(overrides, ret.Interface); err != nil {
				logger.Warnf("auth: could not apply interface overrides for %s: %v", u.Username, err)
			}
		}
	}

	if u.Can(auth.PermManageServer) {
		return ret
	}

	// Redact everything a non-administrator has no business reading. The
	// general section carries filesystem paths, the ffmpeg command lines, the
	// global API key and the local credential hash.
	general := &ConfigGeneralResult{
		VideoFileNamingAlgorithm: ret.General.VideoFileNamingAlgorithm,
		PreviewSegments:          ret.General.PreviewSegments,
		PreviewSegmentDuration:   ret.General.PreviewSegmentDuration,
		PreviewExcludeStart:      ret.General.PreviewExcludeStart,
		PreviewExcludeEnd:        ret.General.PreviewExcludeEnd,
		PreviewPreset:            ret.General.PreviewPreset,
	}
	if u.Can(auth.PermScrape) {
		general.StashBoxes = ret.General.StashBoxes
	} else {
		general.StashBoxes = []*models.StashBox{}
	}
	ret.General = general

	ret.Dlna = &ConfigDLNAResult{}
	ret.Scraping = &ConfigScrapingResult{}
	ret.Defaults = &ConfigDefaultSettingsResult{}
	ret.Plugins = map[string]map[string]interface{}{}

	return ret
}

// scopedUITarget reports whether ui config writes from this caller should go
// to their personal store rather than to the server config, and returns the
// user when so.
func scopedUITarget(ctx context.Context) *auth.User {
	am := authManager()
	if am == nil || !am.Enabled() {
		return nil
	}
	u := auth.FromContext(ctx)
	if u == nil {
		return nil
	}
	if u.Can(auth.PermManageServer) {
		// administrators edit the server-wide defaults
		return nil
	}
	return u
}

// scopedConfigureUI applies a configureUI mutation to the caller's own store.
func scopedConfigureUI(ctx context.Context, u *auth.User, input, partial map[string]interface{}) (map[string]interface{}, error) {
	am := authManager()

	cfg := u.UI()
	if input != nil {
		// a full replacement must not drop the interface overrides
		iface := cfg[userInterfaceKey]
		cfg = map[string]interface{}{}
		for k, v := range input {
			cfg[k] = v
		}
		if iface != nil {
			cfg[userInterfaceKey] = iface
		}
	}
	if partial != nil {
		utils.MergeMaps(cfg, partial)
	}

	if err := am.Users.SetUIConfig(ctx, u.ID, cfg); err != nil {
		return nil, err
	}

	out := map[string]interface{}{}
	for k, v := range cfg {
		if k == userInterfaceKey {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// scopedConfigureInterface stores a configureInterface mutation as the
// caller's personal overrides instead of writing the server config.
func scopedConfigureInterface(ctx context.Context, u *auth.User, input ConfigInterfaceInput, current *ConfigInterfaceResult) (*ConfigInterfaceResult, error) {
	am := authManager()

	overrides := map[string]interface{}{}
	if err := mapstructure.Decode(input, &overrides); err != nil {
		return nil, err
	}
	// drop nils so that an unset field does not clobber an existing override
	for k, v := range overrides {
		if v == nil {
			delete(overrides, k)
		}
	}

	cfg := u.UI()
	existing, _ := cfg[userInterfaceKey].(map[string]interface{})
	if existing == nil {
		existing = map[string]interface{}{}
	}
	utils.MergeMaps(existing, overrides)
	cfg[userInterfaceKey] = existing

	if err := am.Users.SetUIConfig(ctx, u.ID, cfg); err != nil {
		return nil, err
	}

	if err := mapstructure.Decode(existing, current); err != nil {
		return nil, err
	}
	return current, nil
}
