package auth

import "strings"

// Role is a coarse identity class assigned from IdP group membership.
type Role string

const (
	// RoleNone means "authenticated but not authorised for anything".
	RoleNone Role = ""
	// RoleGuest can browse and have playback tracked, nothing else.
	RoleGuest Role = "guest"
	// RoleUser can browse, track activity and personalise their own view.
	RoleUser Role = "user"
	// RoleCurator can additionally edit library metadata and scrape.
	RoleCurator Role = "curator"
	// RoleAdmin can do everything, including server administration.
	RoleAdmin Role = "admin"
)

// AllRoles is the ordered set of roles, least to most privileged.
var AllRoles = []Role{RoleGuest, RoleUser, RoleCurator, RoleAdmin}

func (r Role) Valid() bool {
	for _, v := range AllRoles {
		if v == r {
			return true
		}
	}
	return false
}

// ParseRole is tolerant of case and whitespace. Unknown values yield RoleNone.
func ParseRole(s string) Role {
	r := Role(strings.ToLower(strings.TrimSpace(s)))
	if r.Valid() {
		return r
	}
	return RoleNone
}

// Permission is a single capability that a role may hold.
type Permission string

const (
	// PermViewLibrary allows reading scenes, images, galleries and metadata.
	PermViewLibrary Permission = "view_library"
	// PermTrackActivity allows recording plays, o-counts and resume position.
	PermTrackActivity Permission = "track_activity"
	// PermPersonalise allows writing the caller's own interface config,
	// saved filters, default filters, ratings and favourites.
	PermPersonalise Permission = "personalise"
	// PermEditMetadata allows creating and updating library objects.
	PermEditMetadata Permission = "edit_metadata"
	// PermDeleteContent allows destroying objects and touching files on disk.
	PermDeleteContent Permission = "delete_content"
	// PermScrape allows running scrapers and stash-box operations.
	PermScrape Permission = "scrape"
	// PermRunTasks allows starting and stopping library tasks and jobs.
	PermRunTasks Permission = "run_tasks"
	// PermManageServer allows reading and writing server configuration,
	// plugins, packages, the database and the log stream.
	PermManageServer Permission = "manage_server"
)

// AllPermissions is every permission, in display order.
var AllPermissions = []Permission{
	PermViewLibrary,
	PermTrackActivity,
	PermPersonalise,
	PermEditMetadata,
	PermDeleteContent,
	PermScrape,
	PermRunTasks,
	PermManageServer,
}

var rolePermissions = map[Role]map[Permission]bool{
	RoleGuest: {
		PermViewLibrary:   true,
		PermTrackActivity: true,
	},
	RoleUser: {
		PermViewLibrary:   true,
		PermTrackActivity: true,
		PermPersonalise:   true,
	},
	RoleCurator: {
		PermViewLibrary:   true,
		PermTrackActivity: true,
		PermPersonalise:   true,
		PermEditMetadata:  true,
		PermScrape:        true,
	},
	RoleAdmin: {
		PermViewLibrary:   true,
		PermTrackActivity: true,
		PermPersonalise:   true,
		PermEditMetadata:  true,
		PermDeleteContent: true,
		PermScrape:        true,
		PermRunTasks:      true,
		PermManageServer:  true,
	},
}

// Can reports whether the role holds the given permission.
func (r Role) Can(p Permission) bool {
	perms, ok := rolePermissions[r]
	if !ok {
		return false
	}
	return perms[p]
}

// Permissions returns the role's permission set as a stable slice, suitable
// for shipping to the frontend.
func (r Role) Permissions() []Permission {
	out := make([]Permission, 0, len(AllPermissions))
	for _, p := range AllPermissions {
		if r.Can(p) {
			out = append(out, p)
		}
	}
	return out
}

// queryPermissions maps each root Query field to the permission it requires.
//
// Anything absent from this map falls through to PermManageServer. That is
// deliberate: when upstream adds a new query, it is admin-only until it has
// been classified here, rather than silently exposed.
var queryPermissions = map[string]Permission{
	// library reads
	"findSavedFilter":       PermViewLibrary,
	"findSavedFilters":      PermViewLibrary,
	"findDefaultFilter":     PermViewLibrary,
	"findFile":              PermViewLibrary,
	"findFiles":             PermViewLibrary,
	"findFolder":            PermViewLibrary,
	"findFolders":           PermViewLibrary,
	"findScene":             PermViewLibrary,
	"findSceneByHash":       PermViewLibrary,
	"findScenes":            PermViewLibrary,
	"findScenesByPathRegex": PermViewLibrary,
	"findDuplicateScenes":   PermViewLibrary,
	"sceneStreams":          PermViewLibrary,
	"findSceneMarkers":      PermViewLibrary,
	"findImage":             PermViewLibrary,
	"findImages":            PermViewLibrary,
	"findPerformer":         PermViewLibrary,
	"findPerformers":        PermViewLibrary,
	"findStudio":            PermViewLibrary,
	"findStudios":           PermViewLibrary,
	"findMovie":             PermViewLibrary,
	"findMovies":            PermViewLibrary,
	"findGroup":             PermViewLibrary,
	"findGroups":            PermViewLibrary,
	"findGallery":           PermViewLibrary,
	"findGalleries":         PermViewLibrary,
	"findTag":               PermViewLibrary,
	"findTags":              PermViewLibrary,
	"markerWall":            PermViewLibrary,
	"sceneWall":             PermViewLibrary,
	"markerStrings":         PermViewLibrary,
	"stats":                 PermViewLibrary,
	"sceneMarkerTags":       PermViewLibrary,
	"allScenes":             PermViewLibrary,
	"allSceneMarkers":       PermViewLibrary,
	"allImages":             PermViewLibrary,
	"allGalleries":          PermViewLibrary,
	"allPerformers":         PermViewLibrary,
	"allTags":               PermViewLibrary,
	"allStudios":            PermViewLibrary,
	"allMovies":             PermViewLibrary,
	"version":               PermViewLibrary,
	"me":                    PermViewLibrary,

	// non-admin but privileged
	"parseSceneFilenames":   PermEditMetadata,
	"listScrapers":          PermScrape,
	"scrapeSingleScene":     PermScrape,
	"scrapeMultiScenes":     PermScrape,
	"scrapeSingleStudio":    PermScrape,
	"scrapeSingleTag":       PermScrape,
	"scrapeSinglePerformer": PermScrape,
	"scrapeMultiPerformers": PermScrape,
	"scrapeSingleGallery":   PermScrape,
	"scrapeSingleMovie":     PermScrape,
	"scrapeSingleGroup":     PermScrape,
	"scrapeSingleImage":     PermScrape,
	"scrapeURL":             PermScrape,
	"scrapePerformerURL":    PermScrape,
	"scrapeSceneURL":        PermScrape,
	"scrapeGalleryURL":      PermScrape,
	"scrapeImageURL":        PermScrape,
	"scrapeMovieURL":        PermScrape,
	"scrapeGroupURL":        PermScrape,

	// jobQueue lists every job on the server regardless of who started it or
	// what kind it is -- that's a genuine enumeration of server activity
	// (an admin-only metadataClean run's description/error included), so it
	// stays run_tasks.
	"jobQueue": PermRunTasks,

	// findJob is different from jobQueue even though both are in the "jobs"
	// family: the caller already has the specific job ID, almost always
	// because they got it back from metadataScan (see mutationPermissions
	// below, same reasoning). It isn't a listing/enumeration capability --
	// it's "did the thing I was already trusted to start finish yet" -- so
	// it belongs at the same tier as the mutation that produced the ID, not
	// with jobQueue. Without this, a curator (or a content-adding service
	// account) can kick off a scan but can never find out whether it
	// finished, which file it landed as, or whether it errored.
	"findJob": PermEditMetadata,

	// The configuration query is readable by everyone because it carries the
	// interface and ui settings the frontend needs in order to render at all.
	// The resolver redacts every admin-only section for callers without
	// manage_server -- see internal/api/config_scope.go. Do not tighten this
	// to manage_server without first moving interface/ui to their own query.
	"configuration": PermViewLibrary,

	// Same reasoning as configuration, and just as load-bearing: the
	// frontend's top-level router (App.tsx renderContent()) will not render
	// *any* route -- not even the navbar's page content -- until this query
	// has resolved, because it uses systemStatus.status to detect the
	// needs-migration state and redirect. With this at manage_server, every
	// non-admin session got a permission-denied error here forever and the
	// entire app hung on a bare loading spinner behind a rendered navbar --
	// found the hard way when a stash-users account could sign in but never
	// get past "Loading...". The resolver redacts the server-internal fields
	// (paths, OS, ffmpeg location) for callers without manage_server -- see
	// internal/api/system_status_scope.go.
	"systemStatus": PermViewLibrary,

	// Same reasoning as configuration/systemStatus: the frontend's
	// PluginsLoader fires this on every page load (not just for admins) to
	// inject each enabled plugin's CSS/JS -- that's how a custom theme like
	// fagmob-neon applies to every signed-in user, not just admins. With this
	// at manage_server, a non-admin's session never loads any plugin CSS/JS
	// at all, so the theme silently never appears for anyone else -- found
	// the hard way when a stash-users account loaded with no theme and a
	// console "plugins requires the manage_server permission" error. The
	// resolver (pkg/plugin.Plugin) returns only id/name/version/enabled,
	// task and hook names, and CSS/JS asset paths -- plugin *setting values*
	// (which can hold secrets, e.g. an API key) are a different field
	// entirely (configuration.plugins) and stay redacted to {} for
	// non-admins in config_scope.go. Do not widen this without re-checking
	// pkg/plugin.Plugin hasn't grown a field that leaks settings values.
	"plugins": PermViewLibrary,

	// explicit admin-only reads (also the default, listed for documentation)
	"logs":                        PermManageServer,
	"pluginTasks":                 PermManageServer,
	"installedPackages":           PermManageServer,
	"availablePackages":           PermManageServer,
	"directory":                   PermManageServer,
	"validateStashBoxCredentials": PermManageServer,
	"dlnaStatus":                  PermManageServer,
	"latestversion":               PermManageServer,
}

// mutationPermissions maps each root Mutation field to its required
// permission. Absent entries fall through to PermManageServer (fail closed).
var mutationPermissions = map[string]Permission{
	// per-user activity — safe for every authenticated identity
	"sceneIncrementO":         PermTrackActivity,
	"sceneDecrementO":         PermTrackActivity,
	"sceneAddO":               PermTrackActivity,
	"sceneDeleteO":            PermTrackActivity,
	"sceneResetO":             PermTrackActivity,
	"sceneSaveActivity":       PermTrackActivity,
	"sceneResetActivity":      PermTrackActivity,
	"sceneIncrementPlayCount": PermTrackActivity,
	"sceneAddPlay":            PermTrackActivity,
	"sceneDeletePlay":         PermTrackActivity,
	"sceneResetPlayCount":     PermTrackActivity,
	"imageIncrementO":         PermTrackActivity,
	"imageDecrementO":         PermTrackActivity,
	"imageResetO":             PermTrackActivity,

	// personalisation — writes are redirected to the caller's own namespace
	// by the resolver patches, so these are safe for RoleUser and above.
	"saveFilter":         PermPersonalise,
	"destroySavedFilter": PermPersonalise,
	"setDefaultFilter":   PermPersonalise,
	"configureUI":        PermPersonalise,
	"configureUISetting": PermPersonalise,
	"configureInterface": PermPersonalise,
	"generateOwnApiKey":  PermPersonalise,
	"revokeOwnApiKey":    PermPersonalise,

	// excluding a scene from *your own* random pool is exactly the same
	// tier as favouriting or rating it -- a personal viewing preference,
	// not a change to the library.
	"sceneSetRandomExcluded": PermPersonalise,

	// library metadata editing
	"sceneCreate":             PermEditMetadata,
	"sceneUpdate":             PermEditMetadata,
	"bulkSceneUpdate":         PermEditMetadata,
	"scenesUpdate":            PermEditMetadata,
	"sceneMarkerCreate":       PermEditMetadata,
	"sceneMarkerUpdate":       PermEditMetadata,
	"bulkSceneMarkerUpdate":   PermEditMetadata,
	"sceneAssignFile":         PermEditMetadata,
	"sceneGenerateScreenshot": PermEditMetadata,
	"imageUpdate":             PermEditMetadata,
	"bulkImageUpdate":         PermEditMetadata,
	"imagesUpdate":            PermEditMetadata,
	"galleryCreate":           PermEditMetadata,
	"galleryUpdate":           PermEditMetadata,
	"bulkGalleryUpdate":       PermEditMetadata,
	"galleriesUpdate":         PermEditMetadata,
	"addGalleryImages":        PermEditMetadata,
	"removeGalleryImages":     PermEditMetadata,
	"setGalleryCover":         PermEditMetadata,
	"resetGalleryCover":       PermEditMetadata,
	"galleryChapterCreate":    PermEditMetadata,
	"galleryChapterUpdate":    PermEditMetadata,
	"galleryChapterDestroy":   PermEditMetadata,
	"performerCreate":         PermEditMetadata,
	"performerUpdate":         PermEditMetadata,
	"bulkPerformerUpdate":     PermEditMetadata,
	"studioCreate":            PermEditMetadata,
	"studioUpdate":            PermEditMetadata,
	"bulkStudioUpdate":        PermEditMetadata,
	"movieCreate":             PermEditMetadata,
	"movieUpdate":             PermEditMetadata,
	"bulkMovieUpdate":         PermEditMetadata,
	"groupCreate":             PermEditMetadata,
	"groupUpdate":             PermEditMetadata,
	"bulkGroupUpdate":         PermEditMetadata,
	"addGroupSubGroups":       PermEditMetadata,
	"removeGroupSubGroups":    PermEditMetadata,
	"reorderSubGroups":        PermEditMetadata,
	"tagCreate":               PermEditMetadata,
	"tagUpdate":               PermEditMetadata,
	"bulkTagUpdate":           PermEditMetadata,
	"fileSetFingerprints":     PermEditMetadata,

	// destructive
	"sceneDestroy":              PermDeleteContent,
	"scenesDestroy":             PermDeleteContent,
	"sceneMerge":                PermDeleteContent,
	"sceneMarkerDestroy":        PermDeleteContent,
	"sceneMarkersDestroy":       PermDeleteContent,
	"imageDestroy":              PermDeleteContent,
	"imagesDestroy":             PermDeleteContent,
	"galleryDestroy":            PermDeleteContent,
	"performerDestroy":          PermDeleteContent,
	"performersDestroy":         PermDeleteContent,
	"performerMerge":            PermDeleteContent,
	"studioDestroy":             PermDeleteContent,
	"studiosDestroy":            PermDeleteContent,
	"movieDestroy":              PermDeleteContent,
	"moviesDestroy":             PermDeleteContent,
	"groupDestroy":              PermDeleteContent,
	"groupsDestroy":             PermDeleteContent,
	"tagDestroy":                PermDeleteContent,
	"tagsDestroy":               PermDeleteContent,
	"tagsMerge":                 PermDeleteContent,
	"moveFiles":                 PermDeleteContent,
	"deleteFiles":               PermDeleteContent,
	"destroyFiles":              PermDeleteContent,
	"revealFileInFileManager":   PermManageServer,
	"revealFolderInFileManager": PermManageServer,

	// scraping / stash-box
	"submitStashBoxFingerprints":   PermScrape,
	"submitStashBoxSceneDraft":     PermScrape,
	"submitStashBoxPerformerDraft": PermScrape,
	"stashBoxBatchPerformerTag":    PermScrape,
	"stashBoxBatchStudioTag":       PermScrape,
	"stashBoxBatchTagTag":          PermScrape,

	// metadataScan is how a file that landed on disk outside a GraphQL
	// mutation (an rsync, a download tool, yt-dlp via a bookmarklet-style
	// integration) gets picked up into the library -- it's the mechanism
	// for adding content, not server administration, so it belongs at the
	// same tier as sceneCreate/tagCreate rather than lumped in with the
	// tasks below. A curator who can already create and edit scenes
	// directly gains nothing from being blocked here; they'd just be
	// unable to *finish* adding something they were already trusted to add.
	"metadataScan": PermEditMetadata,

	// tasks: async jobs that are either destructive (metadataClean removes
	// library entries for files that vanished), reach out to external
	// services (metadataIdentify queries stash-box), operate across the
	// whole library regardless of who added what (metadataGenerate,
	// metadataAutoTag), or can interrupt someone else's in-flight job
	// (stopJob/stopAllJobs). These stay admin-only.
	"metadataGenerate":       PermRunTasks,
	"metadataAutoTag":        PermRunTasks,
	"metadataClean":          PermRunTasks,
	"metadataCleanGenerated": PermRunTasks,
	"metadataIdentify":       PermRunTasks,
	"stopJob":                PermRunTasks,
	"stopAllJobs":            PermRunTasks,

	// everything else is server administration and falls through to the
	// default: setup, migrate, downloadFFMpeg, configureGeneral,
	// configureDLNA, configureScraping, configureDefaults, configurePlugin,
	// generateAPIKey, exportObjects, importObjects, metadataImport,
	// metadataExport, migrate*, anonymiseDatabase, optimiseDatabase,
	// reloadScrapers, setPluginsEnabled, runPluginTask, runPluginOperation,
	// reloadPlugins, installPackages, updatePackages, uninstallPackages,
	// backupDatabase, querySQL, execSQL, enableDLNA, disableDLNA,
	// addTempDLNAIP, removeTempDLNAIP.
}

// PermissionForOperation returns the permission required to execute the given
// root field. Unknown fields require PermManageServer.
func PermissionForOperation(opType string, field string) Permission {
	var m map[string]Permission
	switch opType {
	case "query", "subscription":
		m = queryPermissions
	default:
		m = mutationPermissions
	}
	if p, ok := m[field]; ok {
		return p
	}
	return PermManageServer
}

// subscriptionPermissions gates the two streaming endpoints.
var subscriptionPermissions = map[string]Permission{
	"jobsSubscribe":         PermRunTasks,
	"loggingSubscribe":      PermManageServer,
	"scanCompleteSubscribe": PermRunTasks,
}

func init() {
	for k, v := range subscriptionPermissions {
		queryPermissions[k] = v
	}
}
