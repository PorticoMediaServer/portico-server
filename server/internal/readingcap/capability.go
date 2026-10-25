// Package readingcap holds Reading source-format capability facts that several
// independent packages must agree on. CB7 is a build capability, not a runtime
// guess: the portico_cb7 tag must be set, and the platform must be able to
// enforce the bounded archive worker's resource limits.
package readingcap

import "runtime"

// CB7 reports whether this build and platform may expose local CB7 sources.
// The default build is dark.
func CB7() bool { return buildEnabled() && platformSupportsCB7() }

func platformSupportsCB7() bool { return runtime.GOOS == "linux" || runtime.GOOS == "darwin" }

// ComicFormats is every admitted comic source format, in the existing order
// with cb7 appended only when this build exposes it. Existing enum meanings do
// not change.
func ComicFormats() []string {
	formats := []string{"cbz", "pdf"}
	if CB7() {
		formats = append(formats, "cb7")
	}
	return formats
}

// ComicFormatsSQL is the quoted SQL list for ComicFormats.
func ComicFormatsSQL() string {
	return quoted(ComicFormats())
}

// ComicArchiveFormats is the archived comic formats (no PDF, which has no
// archive members).
func ComicArchiveFormats() []string {
	formats := []string{"cbz"}
	if CB7() {
		formats = append(formats, "cb7")
	}
	return formats
}

// ComicArchiveFormatsSQL is the quoted SQL list for ComicArchiveFormats.
func ComicArchiveFormatsSQL() string { return quoted(ComicArchiveFormats()) }

// LocalCB7SQL keeps a cb7 publication selectable only through a current local
// library source of its own library. Remote cb7, or a local copy of the same
// asset token that belongs to another library, neither becomes a primary
// version nor advertises itself as inventory-ready. Availability itself is not
// required here: an unavailable local title keeps the existing browse policy.
// The publication alias is server-owned SQL, never caller text.
func LocalCB7SQL(publication string) string {
	return "(" + publication + ".format<>'cb7' OR EXISTS(SELECT 1" +
		" FROM catalog_entities cb7_entity" +
		" JOIN catalog_libraries cb7_library ON cb7_library.id=cb7_entity.library_id" +
		" JOIN catalog_asset_links cb7_link ON cb7_link.entity_id=" + publication + ".item_id AND cb7_link.asset_id=" + publication + ".asset_id" +
		" JOIN catalog_assets cb7_asset ON cb7_asset.id=" + publication + ".asset_id" +
		" JOIN inventory_objects cb7_object ON cb7_object.asset_id=cb7_asset.token" +
		" JOIN library_sources cb7_source ON cb7_source.id=cb7_object.source_id AND cb7_source.library_id=cb7_library.library_id" +
		" WHERE cb7_entity.id=" + publication + ".item_id" +
		" AND cb7_object.retired=0 AND cb7_object.root_incarnation=cb7_source.incarnation" +
		" AND cb7_object.size=cb7_asset.size AND cb7_object.modified_ns=cb7_asset.modified_ns" +
		" AND cb7_source.kind='local'))"
}

func quoted(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += "'" + value + "'"
	}
	return out
}
