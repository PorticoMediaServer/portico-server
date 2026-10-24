package playbackv1

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/sonic"
)

type sonicWindowSelection func(*layout) ([]int64, error)

func (l *layout) sonicWindowAt(position int64) (int64, bool) {
	if !l.q.shuffle || position < 0 {
		return 0, false
	}
	base := position
	if len(l.spans) > 0 {
		base = -1
		for _, span := range l.spans {
			if position < span.count {
				if span.source {
					return 0, false
				}
				base = span.start + position
				break
			}
			position -= span.count
		}
	}
	return base / sonic.SmoothWindowMax, base >= 0 && base < l.q.domain
}

func (l *layout) sonicWindowsForRange(from, count int64) ([]int64, error) {
	if count < 0 || count > saveStepEntries {
		return nil, ErrQueueTooLarge
	}
	from = max(0, min(from, l.total()))
	count = min(count, l.total()-from)
	seen := map[int64]bool{}
	windows := []int64{}
	for index := int64(0); index < count; index++ {
		if window, ok := l.sonicWindowAt(from + index); ok && !seen[window] {
			seen[window] = true
			windows = append(windows, window)
		}
	}
	return windows, nil
}

// fn must only project the admitted read snapshot. A missing row is captured
// after closing that snapshot, never by upgrading its connection to a writer.
func (s *Service) withMusicLayout(ctx context.Context, caller Caller, id string, selectWindows sonicWindowSelection, fn func(*layout) error) error {
	for attempt := 0; attempt < 3; attempt++ {
		var q queueRow
		missing := []int64{}
		err := func() error {
			tx, done, err := dbwork.BeginRead(ctx, s.DB)
			if err != nil {
				return err
			}
			defer done()
			q, err = loadQueue(ctx, tx, id)
			if err != nil {
				return err
			}
			if !ownsQueue(caller, q) {
				return ErrNotFound
			}
			if err = s.castQueueAuthorityTx(ctx, tx, caller, id); err != nil {
				return err
			}
			l, err := s.readLayout(ctx, tx, q)
			if err != nil {
				return err
			}
			if l.musicQueue() || q.shuffle && len(l.building) > 0 {
				windows, err := selectWindows(l)
				if err != nil {
					return err
				}
				l.sonicWindows = map[int64]sonicWindowCapture{}
				for _, window := range windows {
					key, ordinary, _, _, err := l.sonicWindowSource(window, false)
					if err != nil {
						return err
					}
					captured, err := readSonicWindowTx(ctx, tx, key, ordinary)
					if errors.Is(err, sql.ErrNoRows) {
						missing = append(missing, window)
					} else if err != nil {
						return err
					} else {
						l.sonicWindows[window] = captured
					}
				}
			}
			if len(missing) > 0 {
				return nil
			}
			return fn(l)
		}()
		if err != nil || len(missing) == 0 {
			return err
		}
		for _, window := range missing {
			err = dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassInteractive, func(ctx context.Context, tx *sql.Tx) error {
				fresh, err := loadQueue(ctx, tx, id)
				if err != nil {
					return err
				}
				if !ownsQueue(caller, fresh) {
					return ErrNotFound
				}
				if err = s.castQueueAuthorityTx(ctx, tx, caller, id); err != nil {
					return err
				}
				if fresh.revision != q.revision {
					return ErrRevision
				}
				l, err := s.readLayout(ctx, tx, fresh)
				if err != nil {
					return err
				}
				return s.captureMusicWindowTx(ctx, tx, caller, l, window)
			})
			if errors.Is(err, ErrRevision) {
				break // reselect against the changed queue, within the same bound
			}
			if err != nil {
				return err
			}
		}
	}
	return ErrRevision
}
