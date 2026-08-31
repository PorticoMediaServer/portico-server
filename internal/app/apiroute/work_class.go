package apiroute

import "github.com/PorticoMediaServer/portico-server/internal/foundationcontract"

var operationWorkClass = map[string]foundationcontract.WorkClass{
	// Revocation and authorization fences have isolated request admission.
	"postAuthLogout":                         foundationcontract.WorkClassSecurityFence,
	"deleteAccountSessionsId":                foundationcontract.WorkClassSecurityFence,
	"revokeAutomaticProfileTrusts":           foundationcontract.WorkClassSecurityFence,
	"deleteAuthApiKeysId":                    foundationcontract.WorkClassSecurityFence,
	"revokeNativeSession":                    foundationcontract.WorkClassSecurityFence,
	"removeBrowserAccount":                   foundationcontract.WorkClassSecurityFence,
	"signOutAllBrowserAccounts":              foundationcontract.WorkClassSecurityFence,
	"switchBrowserAccount":                   foundationcontract.WorkClassSecurityFence,
	"changeAccountPassword":                  foundationcontract.WorkClassSecurityFence,
	"deleteAccountProfile":                   foundationcontract.WorkClassSecurityFence,
	"updateAccountProfile":                   foundationcontract.WorkClassSecurityFence,
	"setAccountProfilePin":                   foundationcontract.WorkClassSecurityFence,
	"clearAccountProfilePin":                 foundationcontract.WorkClassSecurityFence,
	"deleteDevicesId":                        foundationcontract.WorkClassSecurityFence,
	"patchUsersId":                           foundationcontract.WorkClassSecurityFence,
	"deleteUsersId":                          foundationcontract.WorkClassSecurityFence,
	"postSystemIdentityReset":                foundationcontract.WorkClassSecurityFence,
	"postRemoteAccessUnclaim":                foundationcontract.WorkClassSecurityFence,
	"deletePlaybackSessionsSessionId":        foundationcontract.WorkClassSecurityFence,
	"revokePlaybackContinuation":             foundationcontract.WorkClassSecurityFence,
	"postLiveTvStreamsChannelIdClose":        foundationcontract.WorkClassSecurityFence,
	"revalidateOfflineDownloadAuthorization": foundationcontract.WorkClassSecurityFence,
	"registerPlaybackReceiver":               foundationcontract.WorkClassSecurityFence,
	"revokePlaybackReceiverAuthorization":    foundationcontract.WorkClassSecurityFence,

	// Admission, initial generation, seek/renegotiation and explicit commands.
	"postPlaybackSessions":                     foundationcontract.WorkClassPlaybackStart,
	"postPlaybackSessionsSessionIdCommand":     foundationcontract.WorkClassPlaybackStart,
	"postPlaybackSessionsSessionIdHandoff":     foundationcontract.WorkClassPlaybackStart,
	"postPlaybackSessionsSessionIdPrepareNext": foundationcontract.WorkClassPlaybackStart,
	"renegotiatePlaybackSession":               foundationcontract.WorkClassPlaybackStart,
	"patchPlaybackSessionsSessionIdQueue":      foundationcontract.WorkClassPlaybackStart,
	"putPlaybackSessionsSessionIdQueue":        foundationcontract.WorkClassPlaybackStart,
	"authorizePlaybackReceiver":                foundationcontract.WorkClassPlaybackStart,
	"handoffPlaybackToReceiver":                foundationcontract.WorkClassPlaybackStart,
	"commitPlaybackReceiverHandoff":            foundationcontract.WorkClassPlaybackStart,
	"postPlaybackCastBootstrap":                foundationcontract.WorkClassPlaybackStart,
	"postPlaybackCastReconnect":                foundationcontract.WorkClassPlaybackStart,
	"postPlaybackCastRedeem":                   foundationcontract.WorkClassPlaybackStart,
	"postPlaybackCastSessionOperation":         foundationcontract.WorkClassPlaybackStart,
	"postLiveTvPlay":                           foundationcontract.WorkClassPlaybackStart,
	"postLiveTvStreamsChannelIdOpen":           foundationcontract.WorkClassPlaybackStart,
	"tuneLibraryChannel":                       foundationcontract.WorkClassPlaybackStart,
	"postDvrRecordingsIdPlayback":              foundationcontract.WorkClassPlaybackStart,

	// User-requested preparation and transfer retain their own resource budget.
	"createDownloadPreparation":      foundationcontract.WorkClassForegroundTransfer,
	"updateDownloadPreparation":      foundationcontract.WorkClassForegroundTransfer,
	"listDownloadPreparations":       foundationcontract.WorkClassForegroundTransfer,
	"getDownloadPreparation":         foundationcontract.WorkClassForegroundTransfer,
	"createDownloadPreparationGrant": foundationcontract.WorkClassForegroundTransfer,
	"removeDownloadPreparation":      foundationcontract.WorkClassForegroundTransfer,
	"getMediaIdDownload":             foundationcontract.WorkClassForegroundTransfer,
	"getMediaIdDownloadOptions":      foundationcontract.WorkClassForegroundTransfer,
}

func workClassForOperation(operationID, ratePolicy string) foundationcontract.WorkClass {
	if class, ok := operationWorkClass[operationID]; ok {
		return class
	}
	switch ratePolicy {
	case "playback-control", "media-delivery":
		return foundationcontract.WorkClassEstablishedPlayback
	default:
		return foundationcontract.WorkClassInteractive
	}
}
