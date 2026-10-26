package remotesources

// Root ruling 57 version 1 maps the four drawn remote choices to one complete
// explicit read and Advanced baseline. These are product defaults approved for
// v1, not measured recommendations. This file is the pure mapping/comparison
// half only: it performs no I/O, writes no schema and decides no applicability.
//
// The source adapter supplies applicability. A field the adapter leaves out is
// inapplicable to that provider: the guarded writer must preserve its saved
// value and the comparison must not count it. This helper never guesses the
// provider set and never treats "not mentioned" as applicable.

// sourceChoiceField is one closed, editable Root57 setting ID. IDs are the
// existing wire names shared by the read profile and the Advanced page.
type sourceChoiceField string

const (
	sourceChoiceScanDepth             sourceChoiceField = "scanDepth"
	sourceChoiceWholeFileReads        sourceChoiceField = "wholeFileReads"
	sourceChoiceConnections           sourceChoiceField = "connections"
	sourceChoiceRequestsPerSecond     sourceChoiceField = "requestsPerSecond"
	sourceChoiceBytesPerSecond        sourceChoiceField = "bytesPerSecond"
	sourceChoiceScanIntervalSeconds   sourceChoiceField = "scanIntervalSeconds"
	sourceChoiceWatchForChanges       sourceChoiceField = "watchForChanges"
	sourceChoiceScanOnStart           sourceChoiceField = "scanOnStart"
	sourceChoiceFullRecheckDays       sourceChoiceField = "fullRecheckDays"
	sourceChoiceDoubleCheckFolders    sourceChoiceField = "doubleCheckFolders"
	sourceChoiceSeekPreviews          sourceChoiceField = "seekPreviews"
	sourceChoiceChapterImages         sourceChoiceField = "chapterImages"
	sourceChoiceWaveforms             sourceChoiceField = "waveforms"
	sourceChoiceLoudness              sourceChoiceField = "loudness"
	sourceChoiceChecksums             sourceChoiceField = "checksums"
	sourceChoiceIntrosMode            sourceChoiceField = "introsMode"
	sourceChoiceCacheBytes            sourceChoiceField = "cacheBytes"
	sourceChoiceCacheFloorBytes       sourceChoiceField = "cacheFloorBytes"
	sourceChoiceDirCacheSeconds       sourceChoiceField = "dirCacheSeconds"
	sourceChoiceReadBufferBytes       sourceChoiceField = "readBufferBytes"
	sourceChoiceConnectTimeoutSeconds sourceChoiceField = "connectTimeoutSeconds"
	sourceChoiceIdleTimeoutSeconds    sourceChoiceField = "idleTimeoutSeconds"
	sourceChoiceRetries               sourceChoiceField = "retries"
	sourceChoiceLowLevelRetries       sourceChoiceField = "lowLevelRetries"
)

// sourceChoiceFieldOrder is the closed comparison order. It is the exact 24
// editable rows: five read-profile rows then the nineteen Advanced rows.
var sourceChoiceFieldOrder = [...]sourceChoiceField{
	sourceChoiceScanDepth,
	sourceChoiceWholeFileReads,
	sourceChoiceConnections,
	sourceChoiceRequestsPerSecond,
	sourceChoiceBytesPerSecond,
	sourceChoiceScanIntervalSeconds,
	sourceChoiceWatchForChanges,
	sourceChoiceScanOnStart,
	sourceChoiceFullRecheckDays,
	sourceChoiceDoubleCheckFolders,
	sourceChoiceSeekPreviews,
	sourceChoiceChapterImages,
	sourceChoiceWaveforms,
	sourceChoiceLoudness,
	sourceChoiceChecksums,
	sourceChoiceIntrosMode,
	sourceChoiceCacheBytes,
	sourceChoiceCacheFloorBytes,
	sourceChoiceDirCacheSeconds,
	sourceChoiceReadBufferBytes,
	sourceChoiceConnectTimeoutSeconds,
	sourceChoiceIdleTimeoutSeconds,
	sourceChoiceRetries,
	sourceChoiceLowLevelRetries,
}

// sourceChoiceDefinition is one immutable normalized v1 row. The eventual
// owner-reserved table is keyed (name, definition_revision) and carries these
// columns explicitly; this value is the source-side copy used by the guarded
// writer and by comparison.
type sourceChoiceDefinition struct {
	Name     string
	Revision int64
	Reading  SourceReadValues
	Advanced SourceAdvanced
}

const sourceChoiceV1Revision int64 = 1

var sourceChoiceDefinitionsV1 = [...]sourceChoiceDefinition{
	{
		Name:     "list",
		Revision: sourceChoiceV1Revision,
		Reading:  SourceReadValues{ScanDepth: "file_list_only", WholeFileReads: false, Connections: 2, RequestsPerSecond: 2, BytesPerSecond: 0},
		Advanced: SourceAdvanced{
			ScanIntervalSeconds: 86400, WatchForChanges: false, ScanOnStart: false, FullRecheckDays: 30, DoubleCheckFolders: false,
			SeekPreviews: false, ChapterImages: false, Waveforms: false, Loudness: false, Checksums: false, IntrosMode: "off",
			CacheBytes: 0, CacheFloorBytes: 2 << 30,
			DirCacheSeconds: 60, ReadBufferBytes: 8 << 20, ConnectTimeoutSeconds: 10, IdleTimeoutSeconds: 30, Retries: 1, LowLevelRetries: 2,
		},
	},
	{
		Name:     "once",
		Revision: sourceChoiceV1Revision,
		Reading:  SourceReadValues{ScanDepth: "basic", WholeFileReads: false, Connections: 4, RequestsPerSecond: 4, BytesPerSecond: 0},
		Advanced: SourceAdvanced{
			ScanIntervalSeconds: 21600, WatchForChanges: false, ScanOnStart: false, FullRecheckDays: 14, DoubleCheckFolders: false,
			SeekPreviews: false, ChapterImages: false, Waveforms: false, Loudness: false, Checksums: false, IntrosMode: "off",
			CacheBytes: 8 << 30, CacheFloorBytes: 2 << 30,
			DirCacheSeconds: 60, ReadBufferBytes: 8 << 20, ConnectTimeoutSeconds: 10, IdleTimeoutSeconds: 30, Retries: 1, LowLevelRetries: 2,
		},
	},
	{
		Name:     "everything",
		Revision: sourceChoiceV1Revision,
		Reading:  SourceReadValues{ScanDepth: "complete", WholeFileReads: true, Connections: 8, RequestsPerSecond: 10, BytesPerSecond: 0},
		Advanced: SourceAdvanced{
			ScanIntervalSeconds: 3600, WatchForChanges: true, ScanOnStart: true, FullRecheckDays: 7, DoubleCheckFolders: true,
			SeekPreviews: true, ChapterImages: true, Waveforms: true, Loudness: true, Checksums: true, IntrosMode: "inherit",
			CacheBytes: 32 << 30, CacheFloorBytes: 2 << 30,
			DirCacheSeconds: 60, ReadBufferBytes: 8 << 20, ConnectTimeoutSeconds: 10, IdleTimeoutSeconds: 30, Retries: 1, LowLevelRetries: 2,
		},
	},
	{
		Name:     "ask",
		Revision: sourceChoiceV1Revision,
		Reading:  SourceReadValues{ScanDepth: "basic", WholeFileReads: false, Connections: 2, RequestsPerSecond: 2, BytesPerSecond: 0},
		Advanced: SourceAdvanced{
			ScanIntervalSeconds: 0, WatchForChanges: false, ScanOnStart: false, FullRecheckDays: 0, DoubleCheckFolders: false,
			SeekPreviews: false, ChapterImages: false, Waveforms: false, Loudness: false, Checksums: false, IntrosMode: "off",
			CacheBytes: 0, CacheFloorBytes: 2 << 30,
			DirCacheSeconds: 60, ReadBufferBytes: 8 << 20, ConnectTimeoutSeconds: 10, IdleTimeoutSeconds: 30, Retries: 1, LowLevelRetries: 2,
		},
	},
}

// sourceChoiceDefinitionAt resolves exactly one immutable version. There is no
// latest/default fallback: an unknown name or revision is absent. The caller
// maps absence to invalid_request.
func sourceChoiceDefinitionAt(name string, revision int64) (sourceChoiceDefinition, bool) {
	for _, definition := range sourceChoiceDefinitionsV1 {
		if definition.Name == name && definition.Revision == revision {
			return definition, true
		}
	}
	return sourceChoiceDefinition{}, false
}

// sourceChoiceApplicability is the exact field set the actual source adapter
// says it applies. A missing field is not written by the guarded writer and is
// not counted. The helper never builds or infers this set.
type sourceChoiceApplicability map[sourceChoiceField]bool

func (a sourceChoiceApplicability) applies(field sourceChoiceField) bool {
	return a[field]
}

// sourceChoiceReadingMatches enforces the accepted body rule for a startChoice:
// the accompanying values must exactly equal the chosen definition's read half.
// No union, merge or silent rewrite is allowed.
func sourceChoiceReadingMatches(definition sourceChoiceDefinition, reading SourceReadValues) bool {
	return reading == definition.Reading
}

// sourceChoiceChangedFields returns the applicable rows whose current value
// differs from the exact recorded definition. The result is already a subset of
// the adapter's applicability set; an empty result is nil, never a fabricated
// zero-change claim for a legacy/incoherent baseline (the caller omits the
// projection entirely in that case).
func sourceChoiceChangedFields(definition sourceChoiceDefinition, applicable sourceChoiceApplicability, reading SourceReadValues, advanced SourceAdvanced) []sourceChoiceField {
	var changed []sourceChoiceField
	for _, field := range sourceChoiceFieldOrder {
		if !applicable.applies(field) {
			continue
		}
		if sourceChoiceDiffers(field, definition, reading, advanced) {
			changed = append(changed, field)
		}
	}
	return changed
}

// sourceChoiceDiffers compares one closed row exactly. It compares raw saved
// values only: never SourceTransport.Effective, never the observed folder
// interval, never an effective server limit and never the applied cache
// projection.
func sourceChoiceDiffers(field sourceChoiceField, definition sourceChoiceDefinition, reading SourceReadValues, advanced SourceAdvanced) bool {
	switch field {
	case sourceChoiceScanDepth:
		return reading.ScanDepth != definition.Reading.ScanDepth
	case sourceChoiceWholeFileReads:
		return reading.WholeFileReads != definition.Reading.WholeFileReads
	case sourceChoiceConnections:
		return reading.Connections != definition.Reading.Connections
	case sourceChoiceRequestsPerSecond:
		return reading.RequestsPerSecond != definition.Reading.RequestsPerSecond
	case sourceChoiceBytesPerSecond:
		return reading.BytesPerSecond != definition.Reading.BytesPerSecond
	case sourceChoiceScanIntervalSeconds:
		return advanced.ScanIntervalSeconds != definition.Advanced.ScanIntervalSeconds
	case sourceChoiceWatchForChanges:
		return advanced.WatchForChanges != definition.Advanced.WatchForChanges
	case sourceChoiceScanOnStart:
		return advanced.ScanOnStart != definition.Advanced.ScanOnStart
	case sourceChoiceFullRecheckDays:
		return advanced.FullRecheckDays != definition.Advanced.FullRecheckDays
	case sourceChoiceDoubleCheckFolders:
		return advanced.DoubleCheckFolders != definition.Advanced.DoubleCheckFolders
	case sourceChoiceSeekPreviews:
		return advanced.SeekPreviews != definition.Advanced.SeekPreviews
	case sourceChoiceChapterImages:
		return advanced.ChapterImages != definition.Advanced.ChapterImages
	case sourceChoiceWaveforms:
		return advanced.Waveforms != definition.Advanced.Waveforms
	case sourceChoiceLoudness:
		return advanced.Loudness != definition.Advanced.Loudness
	case sourceChoiceChecksums:
		return advanced.Checksums != definition.Advanced.Checksums
	case sourceChoiceIntrosMode:
		return advanced.IntrosMode != definition.Advanced.IntrosMode
	case sourceChoiceCacheBytes:
		return advanced.CacheBytes != definition.Advanced.CacheBytes
	case sourceChoiceCacheFloorBytes:
		return advanced.CacheFloorBytes != definition.Advanced.CacheFloorBytes
	case sourceChoiceDirCacheSeconds:
		return advanced.DirCacheSeconds != definition.Advanced.DirCacheSeconds
	case sourceChoiceReadBufferBytes:
		return advanced.ReadBufferBytes != definition.Advanced.ReadBufferBytes
	case sourceChoiceConnectTimeoutSeconds:
		return advanced.ConnectTimeoutSeconds != definition.Advanced.ConnectTimeoutSeconds
	case sourceChoiceIdleTimeoutSeconds:
		return advanced.IdleTimeoutSeconds != definition.Advanced.IdleTimeoutSeconds
	case sourceChoiceRetries:
		return advanced.Retries != definition.Advanced.Retries
	case sourceChoiceLowLevelRetries:
		return advanced.LowLevelRetries != definition.Advanced.LowLevelRetries
	default:
		// The set is closed. An unknown field is reported as changed rather than
		// silently treated as matching an invented baseline.
		return true
	}
}
