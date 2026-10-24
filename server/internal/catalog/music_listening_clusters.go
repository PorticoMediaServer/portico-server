package catalog

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/sonic"
)

// MusicListeningCluster is private selection evidence, not a display label or
// playback grant. Names come from actual credited artists; the original MTT
// head is not mislabeled as the different MSD style-tag head.
type MusicListeningCluster struct {
	Tracks      []string
	Artists     []ContentEntityRef
	CoverAlbums []ContentEntityRef
	Weight      float64
}
type MusicListeningClusters struct {
	EligibleTracks, AnalyzedTracks int
	Model                          *sonic.Identity
	Groups                         []MusicListeningCluster
}
type musicPlayedTrack struct {
	public  string
	vector  sonic.Embedding
	weight  float64
	artists []int64
	album   ContentEntityRef
}

// ListeningClusters reads only this profile's completed plays. Visibility and
// available source facts filter before the bounded history sample, so hidden
// plays cannot crowd permitted tracks out of it. No provider, inference, SQL
// write, scheduler or global artist mean participates in a read.
func (s *Service) ListeningClusters(viewer Viewer, at time.Time) (MusicListeningClusters, error) {
	out := MusicListeningClusters{Groups: []MusicListeningCluster{}}
	actor, ok := viewerFromPersonalKey(viewer.Profile)
	if !ok || actor.Authority == "" || actor.AccountID == "" || actor.ProfileID == "" {
		return out, ErrAdminQuery
	}
	if dbwork.Snapshot(s.Context()) == nil && s.readTx == nil {
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var err error
			out, err = s.WithContext(ctx).ListeningClusters(viewer, at)
			return err
		})
		return out, err
	}
	if at.IsZero() {
		at = s.recommendationNow(time.Time{})
	}
	visible, args := viewer.itemVisibilitySQL("item.id")
	bind := []any{actor.Authority, actor.AccountID, actor.ProfileID, at.UnixMilli()}
	bind = append(bind, args...)
	bind = append(bind, SonicDefaultEnabled(), sonic.MaxClusterPoints, sonic.ModelID, sonic.ModelVersion, sonic.ModelSHA256)
	rows, err := s.read().Query(`WITH recent AS(
 SELECT h.item_id,h.started_ms FROM play_history h INDEXED BY play_history_listen_profile
 JOIN catalog_entities item ON item.id=h.item_id JOIN catalog_libraries library ON library.id=item.library_id
 JOIN libraries configured ON configured.id=library.library_id AND configured.kind='music'
 WHERE h.authority=? AND h.account_id=? AND h.profile_id=? AND h.kind=7 AND h.completed=1 AND h.started_ms<=?
 AND item.kind=7 AND `+visible+` AND `+currentSonicEligibleSQL+`
 AND COALESCE((SELECT enabled FROM library_sonic_settings settings WHERE settings.library_id=library.library_id),?)=1
 AND EXISTS(SELECT 1 FROM catalog_item_availability available WHERE available.entity_id=item.id AND available.available=1 AND available.retired=0)
 ORDER BY h.started_ms DESC,h.id DESC LIMIT ?)
 SELECT item.id,pid(item.public_id),count(*),`+currentSonicVectorSQL+`,`+radioArtistsSQL+`,pid(album_item.public_id),album_item.title
 FROM recent JOIN catalog_entities item ON item.id=recent.item_id
 JOIN catalog_libraries library ON library.id=item.library_id
 JOIN catalog_songs song ON song.entity_id=item.id JOIN catalog_albums album ON album.entity_id=song.album_id
 JOIN catalog_entities album_item ON album_item.id=album.entity_id AND album_item.retired=0
 GROUP BY item.id ORDER BY max(recent.started_ms) DESC,item.id`, bind...)
	if err != nil {
		return out, err
	}
	tracks := map[int64]musicPlayedTrack{}
	points := []sonic.ClusterPoint{}
	for rows.Next() {
		var id int64
		var track musicPlayedTrack
		var data []byte
		var artists string
		if err = rows.Scan(&id, &track.public, &track.weight, &data, &artists, &track.album.ID, &track.album.Name); err != nil {
			break
		}
		out.EligibleTracks++
		track.vector, err = sonic.FromBytes(data)
		if err != nil {
			err = nil
			continue
		}
		if err = json.Unmarshal([]byte(artists), &track.artists); err != nil {
			break
		}
		tracks[id] = track
		points = append(points, sonic.ClusterPoint{ID: id, Vector: track.vector, Weight: track.weight})
	}
	if next := rows.Err(); err == nil {
		err = next
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	out.AnalyzedTracks = len(points)
	if len(points) == 0 {
		return out, nil
	}
	out.Model = &sonic.Identity{ID: sonic.ModelID, Version: sonic.ModelVersion, SHA256: sonic.ModelSHA256}
	// The publisher decides whether its row has enough plays/coverage. These
	// counts and clusters remain actual facts, without a guessed UI threshold.
	limit := min(sonic.MaxClusters, max(1, len(points)/6))
	clusters, err := sonic.ClusterListening(points, limit)
	if err != nil {
		return out, err
	}
	type rankedArtist struct {
		id     int64
		weight float64
	}
	groupArtists := [][]int64{}
	needed := map[int64]bool{}
	for _, cluster := range clusters {
		members := append([]int64(nil), cluster.Members...)
		sort.Slice(members, func(i, j int) bool {
			a, _ := sonic.Cosine(tracks[members[i]].vector, cluster.Center)
			b, _ := sonic.Cosine(tracks[members[j]].vector, cluster.Center)
			if a != b {
				return a > b
			}
			return members[i] < members[j]
		})
		group := MusicListeningCluster{Tracks: []string{}, Artists: []ContentEntityRef{}, CoverAlbums: []ContentEntityRef{}, Weight: cluster.Weight}
		weights := map[int64]float64{}
		albums := map[string]bool{}
		for _, id := range members {
			track := tracks[id]
			group.Tracks = append(group.Tracks, track.public)
			for _, artist := range track.artists {
				if artist > 0 {
					weights[artist] += track.weight
				}
			}
			if !albums[track.album.ID] && len(group.CoverAlbums) < 3 {
				albums[track.album.ID] = true
				group.CoverAlbums = append(group.CoverAlbums, track.album)
			}
		}
		artists := []rankedArtist{}
		for id, weight := range weights {
			artists = append(artists, rankedArtist{id, weight})
		}
		sort.Slice(artists, func(i, j int) bool {
			if artists[i].weight != artists[j].weight {
				return artists[i].weight > artists[j].weight
			}
			return artists[i].id < artists[j].id
		})
		ids := []int64{}
		for _, artist := range artists[:min(2, len(artists))] {
			ids = append(ids, artist.id)
			needed[artist.id] = true
		}
		groupArtists = append(groupArtists, ids)
		out.Groups = append(out.Groups, group)
	}
	ids := []int64{}
	for id := range needed {
		ids = append(ids, id)
	}
	refs := map[int64]ContentEntityRef{}
	if len(ids) > 0 {
		rows, err = s.read().Query(`SELECT item.id,pid(item.public_id),item.title FROM json_each(?) wanted JOIN catalog_entities item ON item.id=wanted.value WHERE item.kind=5 AND item.retired=0`, idsJSON64(ids))
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var id int64
			var ref ContentEntityRef
			if err = rows.Scan(&id, &ref.ID, &ref.Name); err != nil {
				break
			}
			refs[id] = ref
		}
		if next := rows.Err(); err == nil {
			err = next
		}
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	for n, ids := range groupArtists {
		for _, id := range ids {
			if ref, ok := refs[id]; ok {
				out.Groups[n].Artists = append(out.Groups[n].Artists, ref)
			}
		}
	}
	return out, nil
}
