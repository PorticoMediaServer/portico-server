package dvr

import (
	"context"
	"os"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/mediaartifact"
	"time"
)

// CaptureDriver is an internal adapter into confined media work. Success means
// validated closed bytes, not publication. The scheduler owns every DB fence.
type CaptureDriver interface {
	Available() bool
	RetirementAvailable() bool
	Capture(context.Context, CaptureRequest, func(CaptureCheckpoint) error) (CaptureResult, error)
	Recover(context.Context, CaptureRequest, CaptureCheckpoint) (CaptureResult, error)
	Remove(context.Context, livechannels.Owner, string, mediaartifact.Object) error
}
type CaptureRequest struct {
	Recording      Recording
	Owner          livechannels.Owner
	Input          livechannels.Input
	Allocation     livechannels.Allocation
	Generation     int64
	EstimatedBytes int64
	ArtifactKey    string
	PhysicalLock   *os.File
	OnRetired      func()
}
type CaptureCheckpoint struct {
	Object            mediaartifact.Object
	First, Last       time.Time
	Gaps              int
	WriterRetired     bool
	ProducerSucceeded bool
	BoundaryReached   bool
	SourceRevision    int64 // stamped by the scheduler, never accepted from a client
}
type CaptureResult struct {
	CaptureCheckpoint
	Path                              string // canonical private object path; never serialized to a viewer
	Root                              string // this owner's managed Recorded TV root
	Container, VideoCodec, AudioCodec string
	Width, Height                     int
	Duration                          float64
	ModifiedNS                        int64
	Reason                            string
	Complete                          bool
}
