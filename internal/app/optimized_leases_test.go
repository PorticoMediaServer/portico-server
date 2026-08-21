package app

import "testing"

func TestOptimizedArtifactLeaseAndDeletionClaimAreMutuallyExclusive(t *testing.T) {
	server := &Server{}
	release, ok := server.acquireOptimizedArtifactLease("artifact-one")
	if !ok {
		t.Fatal("initial artifact lease was rejected")
	}
	if _, ok := server.claimOptimizedArtifactDeletion("artifact-one"); ok {
		t.Fatal("deletion claimed an actively leased artifact")
	}
	release()
	release() // release is deliberately idempotent
	releaseDelete, ok := server.claimOptimizedArtifactDeletion("artifact-one")
	if !ok {
		t.Fatal("released artifact could not be claimed for deletion")
	}
	if _, ok := server.acquireOptimizedArtifactLease("artifact-one"); ok {
		t.Fatal("new reader entered after deletion claim")
	}
	releaseDelete()
	if release, ok := server.acquireOptimizedArtifactLease("artifact-one"); !ok {
		t.Fatal("artifact remained blocked after deletion claim release")
	} else {
		release()
	}
}
