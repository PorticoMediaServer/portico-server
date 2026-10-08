package notify

import (
	"database/sql"
	"strconv"
)

// This file is the complete producer catalogue. Every notification the server
// can raise is declared here as one exported function with a fixed severity,
// category, action set and dedupe rule, so the set of things a viewer can be
// told is readable in one place and a subsystem cannot quietly invent a new
// notification shape.
//
// Every function takes the caller's *sql.Tx and writes in the caller's
// transaction: a notice about a state change commits with that state change or
// not at all. None of them fan out to a remote service.

// Download outcomes.
const (
	DownloadFinished = "finished"
	DownloadFailed   = "failed"
)

// NotifyDownload is the seam for the downloads workstream.
//
// Call it in the same transaction that records the terminal state of a download,
// once per download, with:
//
//	authority, account, profile — the viewer who asked for the download
//	downloadID                  — the durable download identifier
//	title                       — the item title, already restriction-checked
//	outcome                     — DownloadFinished or DownloadFailed
//	reason                      — a short cause for a failure; "" when finished
//
// The dedupe key is the download identifier, so a download that fails, is
// retried and then succeeds leaves exactly one notice per viewer whose text and
// actions are replaced in place rather than a growing pile. A finished download
// offers "open-download"; a failed one offers "retry-job" alongside it.
//
// The caller owns restriction checking. Pass a title the viewer is allowed to
// see, or pass a generic one; this package does not consult library policy.
func NotifyDownload(tx *sql.Tx, now int64, authority, account, profile, downloadID, title, outcome, reason string) (Result, error) {
	d := Draft{
		Audience:  AudienceProfile,
		Scope:     ProfileScope(authority, account, profile),
		Source:    SourceDownloads,
		DedupeKey: "download:" + downloadID,
		Arguments: map[string]string{"title": title, "downloadId": downloadID},
		Actions: []Action{
			{Kind: "command", Label: "Open", Command: "open-download", Arguments: map[string]string{"downloadId": downloadID}},
		},
	}
	switch outcome {
	case DownloadFinished:
		d.Severity, d.Category = SeverityInfo, "download.finished"
		d.Title = "Download ready"
		d.Body = title + " finished downloading and is ready to play offline."
	case DownloadFailed:
		d.Severity, d.Category = SeverityWarning, "download.failed"
		d.Title = "Download failed"
		d.Body = title + " could not finish downloading."
		if reason != "" {
			d.Body += " " + reason
			d.Arguments["reason"] = reason
		}
		d.Actions = append(d.Actions, Action{Kind: "command", Label: "Try again", Command: "retry-job", Arguments: map[string]string{"downloadId": downloadID}})
	default:
		return Result{}, ErrInvalid
	}
	d.Actions[0].Arguments = map[string]string{"downloadId": downloadID}
	return Raise(tx, now, d)
}

// NotifyRecordingConflict tells the account admins that a scheduled recording
// lost its tuner. The dedupe key is the recording, so a recurring series that
// keeps colliding updates one notice per recording instead of one per attempt.
func NotifyRecordingConflict(tx *sql.Tx, now int64, recordingID, title, reason string) ([]Result, error) {
	body := "Portico could not record " + title + " because no tuner was free."
	if reason == "storage-floor" || reason == "storage-cap" {
		body = "Portico could not record " + title + " because recording storage is full."
	}
	return RaiseForAdmins(tx, now, Draft{
		Severity:  SeverityWarning,
		Source:    SourceDVR,
		Category:  "dvr.conflict",
		DedupeKey: "recording:" + recordingID,
		Title:     "Recording did not start",
		Body:      body,
		Arguments: map[string]string{"title": title, "recordingId": recordingID, "reason": reason},
		Actions: []Action{
			{Kind: "navigate", Label: "Review recordings", Target: &NavigateTarget{View: "recordings", EntityID: recordingID}},
			{Kind: "command", Label: "Dismiss", Command: "dismiss-conflict", Arguments: map[string]string{"recordingId": recordingID}},
		},
	})
}

// Security event kinds.
const (
	SecurityNewDevice      = "new-device"
	SecurityPasswordChange = "password-change"
)

// NotifySecurityEvent goes to the account owner's own profile inbox, because a
// security event is about that person's credentials rather than about server
// administration. A new device is deduped by device identifier so an approval
// retried twice is one notice; a password change is deduped by the millisecond
// it happened, because every change is genuinely a separate event worth seeing.
func NotifySecurityEvent(tx *sql.Tx, now int64, authority, account, profile, kind, deviceID, label string) (Result, error) {
	d := Draft{
		Audience: AudienceProfile,
		Scope:    ProfileScope(authority, account, profile),
		Severity: SeverityWarning,
		Source:   SourceSecurity,
		Actions: []Action{
			{Kind: "navigate", Label: "Review security", Target: &NavigateTarget{View: "settings-security"}},
		},
	}
	switch kind {
	case SecurityNewDevice:
		if deviceID == "" {
			return Result{}, ErrInvalid
		}
		if label == "" {
			label = "A new device"
		}
		d.Category, d.DedupeKey = "security.new-device", "device:"+deviceID
		d.Title = "New device signed in"
		d.Body = label + " was approved for this account. If this was not you, remove it and change your password."
		d.Arguments = map[string]string{"device": label, "deviceId": deviceID}
		d.Actions = append(d.Actions, Action{Kind: "command", Label: "Review device", Command: "review-device", Arguments: map[string]string{"deviceId": deviceID}})
	case SecurityPasswordChange:
		d.Category, d.DedupeKey = "security.password-change", "password:"+strconv.FormatInt(now, 10)
		d.Title = "Account password changed"
		d.Body = "The password for this account was changed and every other signed-in session was ended."
		d.Arguments = map[string]string{}
	default:
		return Result{}, ErrInvalid
	}
	return Raise(tx, now, d)
}

// NotifyScanFailure tells the account admins that a library scan stopped. The
// dedupe key is the source, so a source that keeps failing produces one notice
// whose text and revision move rather than one notice per attempt.
func NotifyScanFailure(tx *sql.Tx, now int64, libraryID, sourceID, code string) ([]Result, error) {
	if sourceID == "" {
		sourceID = libraryID
	}
	return RaiseForAdmins(tx, now, Draft{
		Severity:  SeverityWarning,
		Source:    SourceScan,
		Category:  "scan.failed",
		DedupeKey: "source:" + sourceID,
		Title:     "Library scan stopped",
		Body:      "A scan of this library did not finish (" + code + "). New and changed files are not in the library yet.",
		Arguments: map[string]string{"code": code, "libraryId": libraryID, "sourceId": sourceID},
		Actions: []Action{
			{Kind: "command", Label: "Scan again", Command: "run-scan", Arguments: map[string]string{"libraryId": libraryID}},
			{Kind: "navigate", Label: "Open library", Target: &NavigateTarget{View: "library", EntityID: libraryID, LibraryID: libraryID}},
		},
	})
}

// NotifyCertificateExpiry tells the account admins that the server's TLS
// certificate is running out. One notice per certificate scope, restated (and
// escalated from warning to critical) as the deadline approaches.
func NotifyCertificateExpiry(tx *sql.Tx, now int64, scope string, daysRemaining int) ([]Result, error) {
	if scope == "" {
		scope = "server"
	}
	d := Draft{
		Severity:  SeverityWarning,
		Source:    SourceNetworking,
		Category:  "networking.certificate-expiring",
		DedupeKey: "certificate:" + scope,
		Title:     "Certificate expiring",
		Body:      "The secure connection certificate for this server expires in " + strconv.Itoa(daysRemaining) + " days. Clients will refuse to connect once it does.",
		Arguments: map[string]string{"scope": scope, "daysRemaining": strconv.Itoa(daysRemaining)},
		Actions: []Action{
			{Kind: "navigate", Label: "Open network settings", Target: &NavigateTarget{View: "settings-network"}},
		},
	}
	if daysRemaining <= 0 {
		d.Severity, d.Category = SeverityCritical, "networking.certificate-expired"
		d.Title = "Certificate expired"
		d.Body = "The secure connection certificate for this server has expired. Clients cannot connect until it is renewed."
	}
	return RaiseForAdmins(tx, now, d)
}

// NotifyStorageWarning tells the account admins that the state volume is filling
// up. Severity is chosen by the caller from the configured thresholds, and the
// single dedupe key means the notice escalates in place from warning to critical.
func NotifyStorageWarning(tx *sql.Tx, now int64, severity string, freePercent int, freeBytes int64) ([]Result, error) {
	if severity != SeverityWarning && severity != SeverityCritical {
		return nil, ErrInvalid
	}
	title, body := "Storage running low", "The Portico state volume has "+strconv.Itoa(freePercent)+"% free. Scans, metadata and conversions slow down as it fills."
	if severity == SeverityCritical {
		title, body = "Storage nearly full", "The Portico state volume has "+strconv.Itoa(freePercent)+"% free. Scans, metadata, recordings and conversions fail when it runs out."
	}
	return RaiseForAdmins(tx, now, Draft{
		Severity:  severity,
		Source:    SourceStorage,
		Category:  "storage.low",
		DedupeKey: "volume:state",
		Title:     title,
		Body:      body,
		Arguments: map[string]string{"freePercent": strconv.Itoa(freePercent), "freeBytes": strconv.FormatInt(freeBytes, 10)},
		Actions: []Action{
			{Kind: "navigate", Label: "Open storage settings", Target: &NavigateTarget{View: "settings-storage"}},
		},
	})
}

// NotifyFeedbackSubmitted tells the account admins that a viewer filed a report.
// The report text never enters the notice: an admin opens the report to read it,
// which keeps the inbox out of the reporter's privacy surface.
func NotifyFeedbackSubmitted(tx *sql.Tx, now int64, reportID, kind, category string) ([]Result, error) {
	return RaiseForAdmins(tx, now, Draft{
		Severity:  SeverityInfo,
		Source:    SourceFeedback,
		Category:  "feedback.received",
		DedupeKey: "report:" + reportID,
		Title:     "New viewer feedback",
		Body:      "A viewer reported a problem and it is waiting for review.",
		Arguments: map[string]string{"reportId": reportID, "kind": kind, "category": category},
		Actions: []Action{
			{Kind: "navigate", Label: "Open report", Target: &NavigateTarget{View: "admin-feedback", EntityID: reportID}},
		},
	})
}

// NotifyFeedbackUpdated tells the reporter that their report moved. Deduped by
// report, so a report triaged three times shows one notice at its latest state.
func NotifyFeedbackUpdated(tx *sql.Tx, now int64, authority, account, profile, reportID, status string) (Result, error) {
	return Raise(tx, now, Draft{
		Audience:  AudienceProfile,
		Scope:     ProfileScope(authority, account, profile),
		Severity:  SeverityInfo,
		Source:    SourceFeedback,
		Category:  "feedback.updated",
		DedupeKey: "report:" + reportID,
		Title:     "Your report has an update",
		Body:      "The report you sent is now marked " + status + ".",
		Arguments: map[string]string{"reportId": reportID, "status": status},
		Actions: []Action{
			{Kind: "command", Label: "Open report", Command: "open-feedback", Arguments: map[string]string{"reportId": reportID}},
		},
	})
}
