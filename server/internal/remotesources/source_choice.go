package remotesources

// Root ruling 57 version 1 maps the four drawn remote choices to one complete
// explicit read and Advanced baseline. The normalized definition table is the
// sole values source; this file holds only the closed comparison and the
// adapter-supplied applicability mapping. It performs no SQL and writes nothing.
//
// A field the actual source adapter leaves out is inapplicable: the guarded
// writer preserves its saved value and the comparison does not count it. This
// helper never guesses the provider set and never treats "not mentioned" as
// applicable.

// sourceChoiceApplicability is the exact field set the actual source adapter
// applies. The projection and writer both use the same derived set.
type sourceChoiceApplicability map[string]bool

// The six transport knobs are consumed by the mounted rclone serve program,
// not by the direct native DAV adapter. SourceTransport is explicitly the
// mounted program's transport subset; service.dav(), acquisition.go's direct
// DAV path and listDAVFolders never read them. A source with no mount therefore
// preserves these values and does not count them as part of a choice.
var sourceChoiceTransportFields = [...]string{
	"dirCacheSeconds",
	"readBufferBytes",
	"connectTimeoutSeconds",
	"idleTimeoutSeconds",
	"retries",
	"lowLevelRetries",
}

func sourceChoiceApplicabilityForSource(source Source) sourceChoiceApplicability {
	applicable := make(sourceChoiceApplicability, len(sourceChoiceWireFields))
	for _, field := range sourceChoiceWireFields {
		applicable[field] = true
	}
	if source.MountID == "" {
		for _, field := range sourceChoiceTransportFields {
			delete(applicable, field)
		}
	}
	return applicable
}

// sourceChoiceApplicableFields returns the adapter's set in the closed authored
// order, suitable for SourceChoiceBaseline.ApplicableFields.
func sourceChoiceApplicableFields(source Source) []string {
	applicable := sourceChoiceApplicabilityForSource(source)
	fields := make([]string, 0, len(sourceChoiceWireFields))
	for _, field := range sourceChoiceWireFields {
		if applicable.applies(field) {
			fields = append(fields, field)
		}
	}
	return fields
}

func (a sourceChoiceApplicability) applies(field string) bool {
	return a[field]
}

// sourceChoiceReadingMatches enforces the accepted body rule: the complete
// legacy values must exactly equal the chosen definition's read half. No union,
// merge or silent rewrite is allowed.
func sourceChoiceReadingMatches(definition SourceChoiceDefinition, reading SourceReadValues) bool {
	return reading == definition.Reading
}

// sourceChoicePreserveInapplicable keeps the current saved value for a field
// the adapter does not apply. The values struct is otherwise the exact SQL
// definition.
func sourceChoicePreserveInapplicable(values, current SourceAdvanced, applicable sourceChoiceApplicability) SourceAdvanced {
	if !applicable.applies("dirCacheSeconds") {
		values.DirCacheSeconds = current.DirCacheSeconds
	}
	if !applicable.applies("readBufferBytes") {
		values.ReadBufferBytes = current.ReadBufferBytes
	}
	if !applicable.applies("connectTimeoutSeconds") {
		values.ConnectTimeoutSeconds = current.ConnectTimeoutSeconds
	}
	if !applicable.applies("idleTimeoutSeconds") {
		values.IdleTimeoutSeconds = current.IdleTimeoutSeconds
	}
	if !applicable.applies("retries") {
		values.Retries = current.Retries
	}
	if !applicable.applies("lowLevelRetries") {
		values.LowLevelRetries = current.LowLevelRetries
	}
	return values
}

// sourceChoiceChangedFields returns the applicable rows whose current value
// differs from the exact recorded definition. The result is already a subset of
// the adapter's applicability set; an empty result is nil. The caller omits the
// whole baseline for an absent/incoherent association rather than fabricating a
// zero-change claim.
func sourceChoiceChangedFields(definition SourceChoiceDefinition, applicable sourceChoiceApplicability, reading SourceReadValues, advanced SourceAdvanced) []string {
	var changed []string
	for _, field := range sourceChoiceWireFields {
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
func sourceChoiceDiffers(field string, definition SourceChoiceDefinition, reading SourceReadValues, advanced SourceAdvanced) bool {
	switch field {
	case "scanDepth":
		return reading.ScanDepth != definition.Reading.ScanDepth
	case "wholeFileReads":
		return reading.WholeFileReads != definition.Reading.WholeFileReads
	case "connections":
		return reading.Connections != definition.Reading.Connections
	case "requestsPerSecond":
		return reading.RequestsPerSecond != definition.Reading.RequestsPerSecond
	case "bytesPerSecond":
		return reading.BytesPerSecond != definition.Reading.BytesPerSecond
	case "scanIntervalSeconds":
		return advanced.ScanIntervalSeconds != definition.Advanced.ScanIntervalSeconds
	case "watchForChanges":
		return advanced.WatchForChanges != definition.Advanced.WatchForChanges
	case "scanOnStart":
		return advanced.ScanOnStart != definition.Advanced.ScanOnStart
	case "fullRecheckDays":
		return advanced.FullRecheckDays != definition.Advanced.FullRecheckDays
	case "doubleCheckFolders":
		return advanced.DoubleCheckFolders != definition.Advanced.DoubleCheckFolders
	case "seekPreviews":
		return advanced.SeekPreviews != definition.Advanced.SeekPreviews
	case "chapterImages":
		return advanced.ChapterImages != definition.Advanced.ChapterImages
	case "waveforms":
		return advanced.Waveforms != definition.Advanced.Waveforms
	case "loudness":
		return advanced.Loudness != definition.Advanced.Loudness
	case "checksums":
		return advanced.Checksums != definition.Advanced.Checksums
	case "introsMode":
		return advanced.IntrosMode != definition.Advanced.IntrosMode
	case "cacheBytes":
		return advanced.CacheBytes != definition.Advanced.CacheBytes
	case "cacheFloorBytes":
		return advanced.CacheFloorBytes != definition.Advanced.CacheFloorBytes
	case "dirCacheSeconds":
		return advanced.DirCacheSeconds != definition.Advanced.DirCacheSeconds
	case "readBufferBytes":
		return advanced.ReadBufferBytes != definition.Advanced.ReadBufferBytes
	case "connectTimeoutSeconds":
		return advanced.ConnectTimeoutSeconds != definition.Advanced.ConnectTimeoutSeconds
	case "idleTimeoutSeconds":
		return advanced.IdleTimeoutSeconds != definition.Advanced.IdleTimeoutSeconds
	case "retries":
		return advanced.Retries != definition.Advanced.Retries
	case "lowLevelRetries":
		return advanced.LowLevelRetries != definition.Advanced.LowLevelRetries
	default:
		// The set is closed. An unknown field is reported as changed rather than
		// silently treated as matching an invented baseline.
		return true
	}
}
