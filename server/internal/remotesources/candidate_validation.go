package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"strconv"
	"sync"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

type sourceTestScope struct {
	done   chan struct{}
	life   context.Context
	cancel context.CancelFunc
}

// Source tests queue new operations even when the source has no live process.
// An existing process keeps its exact epoch and readers; only its operation
// budget drains. No physical reader/session is force-retired for a test.
func (s *Service) waitSourceTest(ctx context.Context, source string) (bool, error) {
	s.serveMu.Lock()
	scope := s.sourceTests[source]
	s.serveMu.Unlock()
	if scope == nil {
		return false, nil
	}
	select {
	case <-scope.done:
		return true, nil
	case <-ctx.Done():
		return true, ctx.Err()
	}
}

func (s *Service) beginCandidateTest(ctx context.Context, current *mounts.Mount, limits ReadLimits) (*remotemedia.ExclusiveTest, func(), error) {
	source := identity.Token()
	if current != nil {
		source = current.ID
	}
	s.serveMu.Lock()
	if s.sourceTests == nil {
		s.sourceTests = map[string]*sourceTestScope{}
	}
	if s.serveClosed {
		s.serveMu.Unlock()
		return nil, nil, storage.ErrRemoteOffline
	}
	if s.sourceTests[source] != nil {
		s.serveMu.Unlock()
		return nil, nil, storage.ErrBusy
	}
	life, cancel := context.WithCancel(ctx)
	scope := &sourceTestScope{done: make(chan struct{}), life: life, cancel: cancel}
	s.sourceTests[source] = scope
	slot := s.serveLeases[source]
	var process *rcloneprogram.ServeProcess
	if slot != nil {
		process = slot.process
	}
	s.serveMu.Unlock()
	var once sync.Once
	finish := func() {
		once.Do(func() {
			scope.cancel()
			s.serveMu.Lock()
			if s.sourceTests[source] == scope {
				delete(s.sourceTests, source)
			}
			close(scope.done)
			s.serveMu.Unlock()
		})
	}
	if slot != nil && process == nil {
		finish()
		return nil, nil, storage.ErrBusy
	}
	budget := remotemedia.NewOperationBudget(life, limits.Connections)
	if process != nil {
		var err error
		budget, err = process.SharedOperations(ctx)
		if err != nil {
			finish()
			return nil, nil, err
		}
		if budget.Counts().Capacity != limits.Connections {
			finish()
			return nil, nil, storage.ErrRemoteChanged
		}
	}
	hold, err := budget.AcquireTest(life)
	if err != nil {
		finish()
		if errors.Is(err, remotemedia.ErrBusy) {
			err = storage.ErrBusy
		}
		return nil, nil, err
	}
	return hold, func() { hold.Close(); finish() }, nil
}

type candidateServeLease struct {
	life  context.Context
	check func(context.Context) error
}

func (l *candidateServeLease) Context() context.Context { return l.life }
func (l *candidateServeLease) Check(ctx context.Context) error {
	if err := l.life.Err(); err != nil {
		return err
	}
	return l.check(ctx)
}
func (l *candidateServeLease) Release() {}

func (s *Service) candidateProfile(ctx context.Context, current *mounts.Mount) (SourceReadProfile, error) {
	if current == nil {
		p, err := SourceReadPreset("list")
		return p, err
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return SourceReadProfile{}, err
	}
	defer snapshot.Rollback()
	return SourceReadProfileTx(ctx, snapshot.Tx(), current.ID)
}

func (s *Service) testCandidate(ctx context.Context, input mounts.SourceCandidateInput, raw []byte, program rcloneprogram.Program, digest string, approvals []remotemedia.Approval, profile SourceReadProfile, authorize func(*sql.Tx) error) error {
	var err error
	check := func(ctx context.Context) error {
		snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
		if err != nil {
			return err
		}
		defer snapshot.Rollback()
		if err := authorize(snapshot.Tx()); err != nil {
			return err
		}
		if input.Current != nil {
			current := input.Current
			var revision, generation int64
			var root string
			if err := snapshot.Tx().QueryRowContext(ctx, `SELECT c.revision,b.generation,m.mount_path FROM managed_mounts m JOIN mount_controls c ON c.mount_id=m.id JOIN mount_backend_configs b ON b.mount_id=m.id WHERE m.id=? AND c.removal_pending=0`, current.ID).Scan(&revision, &generation, &root); err != nil {
				return err
			}
			actual, err := SourceReadProfileTx(ctx, snapshot.Tx(), current.ID)
			if err != nil {
				return err
			}
			if revision != current.ControlRevision || generation != current.BackendRevision || root != current.MountPath || actual != profile {
				return storage.ErrRemoteChanged
			}
		}
		snapshot.Rollback()
		return s.checkImportedApprovals(ctx, importedSourceConfiguration{Approvals: approvals})
	}
	if err = check(ctx); err != nil {
		return err
	}
	hold, finish, err := s.beginCandidateTest(ctx, input.Current, profile.Limits)
	if err != nil {
		return err
	}
	defer finish()
	life := hold.Context()
	policy := remotemedia.Policy{Resolver: s.resolver, SourceEndpoints: true}
	for _, a := range approvals {
		if len(a.Root) >= 4 && a.Root[:4] == "http" {
			policy.Approvals = append(policy.Approvals, a)
		}
	}
	proxy, err := remotemedia.StartPolicyProxy(life, policy, identity.Token(), identity.Token(), check)
	if err != nil {
		return err
	}
	defer func() { proxy.Close(); _ = proxy.Wait(context.Background()) }()
	port, err := privateServePort()
	if err != nil {
		return err
	}
	metadata, err := privateServePort()
	if err != nil {
		return err
	}
	source, generation := identity.Token(), "1"
	if input.Current != nil {
		source = input.Current.ID
		generation = strconv.FormatInt(input.Current.BackendRevision, 10)
	}
	spec := rcloneprogram.ServeCommandSpec{SourceID: source, SourceGeneration: generation, ApprovedProgramDigest: digest, PrivateDirectory: s.private, Configuration: raw, Remote: input.Remote, Port: port, MetadataPort: metadata, Username: identity.Token(), Password: identity.Token(), Connections: profile.Limits.Connections, RequestsPerSecond: profile.Limits.RequestsPerSecond, BytesPerSecond: profile.Limits.BytesPerSecond, SharedOperations: hold.Operations(), Enforce: func(cmd *exec.Cmd) error {
		if err := check(life); err != nil {
			return err
		}
		cmd.Env = append(cmd.Env, "RCLONE_HTTP_PROXY="+proxy.Locator(), "RCLONE_SFTP_HTTP_PROXY=")
		return nil
	}}
	process, err := rcloneprogram.StartServe(life, program, spec, &candidateServeLease{life: life, check: check})
	if err != nil {
		if life.Err() != nil {
			return life.Err()
		}
		return mapError(err)
	}
	defer func() { process.Stop(); _ = process.Wait(context.Background()) }()
	if err = check(life); err != nil {
		return err
	}
	metadataClient, err := process.Metadata(life)
	if err != nil {
		return err
	}
	if err = metadataClient.CheckRoot(life, check); err != nil {
		return mapError(err)
	}
	leaves, err := configuredBackends(raw, input.Remote)
	if err != nil {
		return err
	}
	checked := map[string]bool{input.Remote: true}
	for _, leaf := range leaves {
		if checked[leaf.Remote] {
			continue
		}
		checked[leaf.Remote] = true
		client, err := remotemedia.NewRcloneMetadata(metadata, spec.Username, spec.Password, hold.Operations(), leaf.Remote)
		if err != nil {
			return err
		}
		err = client.CheckRoot(life, check)
		client.Close()
		if err != nil {
			if life.Err() != nil {
				return life.Err()
			}
			return mapError(err)
		}
	}
	return check(life)
}
