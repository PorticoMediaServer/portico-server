package catalog

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Continue Watching has a configured maximum, so it only needs enough recent
// started shows to fill that maximum. If restrictions or completed shows remove
// candidates, expand the indexed activity window until it fills or all started
// shows have been considered. The final list is frozen into this request's
// source so its total, cursor and page refer to the same selected episodes.
func (s *Service) homeContinueSource(r HomeRequest, libraries, cutoffs, premieres string, maximum int) (homeSource, error) {
	if maximum < 1 {
		maximum = 1
	}
	movies := `SELECT id,ord,entity FROM (` + homeContinueBase("continue_movies") + `) m
		WHERE m.ord>=(SELECT value FROM json_each(?) WHERE key=(SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(m.id)))`
	restriction, bound := ItemRestrictionSQL("m.entity", r.Restrictions)
	movieArgs := append([]any{r.Profile, libraries, cutoffs}, bound...)
	movieArgs = append(movieArgs, maximum)
	movieRows, err := s.homeCandidates(`SELECT m.id,m.ord FROM (`+movies+`) m WHERE `+restriction+` ORDER BY m.ord DESC,m.id DESC LIMIT ?`, movieArgs...)
	if err != nil {
		return homeSource{}, err
	}
	// Walk the started shows newest first in slices that continue from the
	// previous slice's last key, so each show is evaluated once, and stop as
	// soon as the maximum is filled: every later show's slot is older than every
	// slot already chosen (B83). The walk never counts the profile's history.
	shows := []homeCandidate{}
	var after *showActivityKey
	for slice := maximum; len(shows) < maximum; slice = min(slice*2, 1024) {
		query, args := homeShowContinuation(r, cutoffs, premieres, slice, "", after)
		found, err := s.homeCandidates(`SELECT id,ord FROM (`+query+`)`, args...)
		if err != nil {
			return homeSource{}, err
		}
		shows = append(shows, found...)
		if len(shows) >= maximum {
			break
		}
		next, err := s.showActivityAfter(r.Profile, libraries, after, slice)
		if errors.Is(err, sql.ErrNoRows) {
			break // every started show has been considered
		}
		if err != nil {
			return homeSource{}, err
		}
		after = &next
	}
	selected := mergeHome(movieRows, shows, maximum)
	values := make([][2]string, len(selected))
	for i, candidate := range selected {
		values[i] = [2]string{candidate.id, candidate.order}
	}
	raw, _ := json.Marshal(values)
	n := len(values)
	return homeSource{
		base:           `SELECT json_extract(value,'$[0]') AS id,json_extract(value,'$[1]') AS ord FROM json_each(?)`,
		args:           []any{string(raw)},
		total:          &n,
		selfRestricted: true,
		fingerprint:    fmt.Sprintf("%x", sha256.Sum256(raw)),
	}, nil
}

// showActivityAfter is the key of the last show in the slice of size n that
// starts after the given key (the n-th newest from there), or sql.ErrNoRows
// when the slice held fewer than n shows. It reads n index entries.
func (s *Service) showActivityAfter(profile, libraries string, after *showActivityKey, n int) (showActivityKey, error) {
	query := `SELECT last_activity,show_id FROM profile_show_activity WHERE profile_id=? AND library_id IN(SELECT value FROM json_each(?))`
	args := []any{profile, libraries}
	if after != nil {
		query += ` AND (last_activity<? OR last_activity=? AND show_id>?)`
		args = append(args, after.lastActivity, after.lastActivity, after.showID)
	}
	args = append(args, n-1)
	var key showActivityKey
	err := s.read().QueryRow(query+` ORDER BY last_activity DESC,show_id LIMIT 1 OFFSET ?`, args...).Scan(&key.lastActivity, &key.showID)
	return key, err
}
