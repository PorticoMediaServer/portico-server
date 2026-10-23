package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

const browseSourceVersion = "browse-source-v1:"

// browseSourceFence binds authority and canonical membership separately from
// ordering and decoration revisions. One ordered read covers the scoped
// libraries; no title or member is queried to build this fence.
func (s *Service) browseSourceFence(server string, viewer Viewer, query *BrowseNode, rows map[string][]int64, sources map[string]HomeRowMembership) (string, error) {
	libraries := append([]string(nil), viewer.Libraries...)
	sort.Strings(libraries)
	unique := libraries[:0]
	for _, id := range libraries {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	type libraryState struct {
		ID       string
		Handle   int64
		Retired  int
		Agent    string
		Revision int64
	}
	state := []libraryState{}
	found, err := s.read().Query(`SELECT l.id,COALESCE(cl.id,0),COALESCE(cl.retired,1),COALESCE(a.agent,'online'),COALESCE(a.revision,0)
 FROM libraries l LEFT JOIN catalog_libraries cl ON cl.library_id=l.id LEFT JOIN library_metadata_agents a ON a.library_id=l.id
 WHERE l.id IN(SELECT value FROM json_each(?)) ORDER BY l.id`, idsJSON(unique))
	if err != nil {
		return "", err
	}
	for found.Next() {
		var item libraryState
		if err = found.Scan(&item.ID, &item.Handle, &item.Retired, &item.Agent, &item.Revision); err != nil {
			found.Close()
			return "", err
		}
		state = append(state, item)
	}
	err = found.Err()
	found.Close()
	if err != nil {
		return "", err
	}
	group, err := s.browseGroupFence(query, rows, sources)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(struct {
		Server, Profile, Viewer, Restrictions string
		Libraries                             []string
		State                                 []libraryState
		Group                                 string
	}{server, viewer.Profile, viewer.Fence, RestrictionFence(viewer.EffectiveRestrictions()), unique, state, group})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return browseSourceVersion + hex.EncodeToString(digest[:]), nil
}

// Only browse routes opt into this policy. Generic search/detail cursors keep
// their existing decoder, expiry and revision semantics.
func validateBrowseCursor(cursor *cursorValue, hard, revision string) error {
	if !strings.HasPrefix(cursor.Fingerprint, browseSourceVersion) || cursor.CacheRevision == "" {
		return ErrCursor
	}
	if cursor.Fingerprint != hard {
		return ErrStaleContinuation
	}
	if cursor.CacheRevision != revision {
		cursor.Total = 0
	}
	return nil
}

func browseRequestViewer(request BrowseRequest) Viewer {
	if len(request.viewer.Libraries) > 0 {
		return request.viewer
	}
	// The legacy library browse surface passes its already authorized scope,
	// rather than the richer Viewer. This does not grant any query authority.
	return Viewer{Profile: request.Profile, Fence: request.ViewerFence, Libraries: []string{request.Library}, Restrictions: request.Restrictions}
}
