package app

import "github.com/PorticoMediaServer/portico-server/internal/foundationcontract"

// durableJobDescriptor is the single declaration for a durable operation's
// semantic Foundation class and physical execution governor. WorkClass is
// persisted with each job; ResourceLane remains process-local because CPU,
// disk, metadata-provider, and SQLite limits are not interchangeable.
type durableJobDescriptor struct {
	Type         string
	WorkClass    foundationcontract.WorkClass
	ResourceLane string
	ActiveKey    bool
	Singleton    bool
}

var durableJobDescriptors = []durableJobDescriptor{
	{Type: "library_scan", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneWriteHeavy, ActiveKey: true, Singleton: true},
	{Type: "library_change_check", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneWriteHeavy, ActiveKey: true, Singleton: true},
	{Type: "live_tv_refresh", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneWriteHeavy, ActiveKey: true, Singleton: true},
	{Type: "metadata_refresh", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneMetadata, ActiveKey: true, Singleton: true},
	{Type: "metadata_refresh_library", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneMetadata, ActiveKey: true, Singleton: true},
	{Type: "lyrics_fetch_missing", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneMetadata, ActiveKey: true, Singleton: true},
	{Type: "tmdb_trending_refresh", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneMetadata, ActiveKey: true, Singleton: true},
	{Type: "media_analyze", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneAnalysis, ActiveKey: true},
	{Type: downloadArtifactVerificationJobType, WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneAnalysis, ActiveKey: true},
	{Type: "optimize_version", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneOptimized, ActiveKey: true},
	{Type: "dashboard_rollup_refresh", WorkClass: foundationcontract.WorkClassBackgroundMedia, ResourceLane: jobLaneBackground, ActiveKey: true},
	{Type: "library_read_model_repair", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
	{Type: remoteAccessCertificateMaintenanceJobType, WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
	{Type: "database_backup", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true},
	{Type: "library_trash_cleanup", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
	{Type: "optimized_version_prune", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
	{Type: "trickplay_prune", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
	{Type: "dvr_retention_cleanup", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
	{Type: "system_storage_cleanup", WorkClass: foundationcontract.WorkClassMaintenance, ResourceLane: jobLaneMaintenance, ActiveKey: true, Singleton: true},
}

func durableJobDescriptorForType(jobType string) (durableJobDescriptor, bool) {
	for _, descriptor := range durableJobDescriptors {
		if descriptor.Type == jobType {
			return descriptor, true
		}
	}
	return durableJobDescriptor{}, false
}

func jobLaneForType(jobType string) string {
	if descriptor, ok := durableJobDescriptorForType(jobType); ok {
		return descriptor.ResourceLane
	}
	return ""
}

func jobTypesForLane(lane string) []string {
	types := []string{}
	for _, descriptor := range durableJobDescriptors {
		if descriptor.ResourceLane == lane {
			types = append(types, descriptor.Type)
		}
	}
	return types
}

func supportedJobType(jobType string) bool {
	_, ok := durableJobDescriptorForType(jobType)
	return ok
}

func jobTypeUsesActiveKey(jobType string) bool {
	descriptor, ok := durableJobDescriptorForType(jobType)
	return ok && descriptor.ActiveKey
}

func maintenanceJobIsSingletonForResource(jobType string) bool {
	descriptor, ok := durableJobDescriptorForType(jobType)
	return ok && descriptor.Singleton
}

func isMaintenanceJobType(jobType string) bool {
	descriptor, ok := durableJobDescriptorForType(jobType)
	return ok && descriptor.ResourceLane != jobLaneBackground
}
