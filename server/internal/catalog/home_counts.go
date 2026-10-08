package catalog

import (
	"portico.local/server/internal/compactcatalog"
	"strings"

	"portico.local/server/internal/identity"
)

// `Recently Added in Films (620,004)` is a number a client renders, so it has to
// be exact. It was also, until this file, a `count(*)` over the whole library
// with a visibility probe on every row — once per library, on every home
// request, for every viewer.
//
// `home_item_buckets` (internal/persistence/browse_rows.go) counts the same rows
// the browse buckets count, along the axes a home row asks about instead of the
// letter axis a browse rail asks about: whether the item is visible, whether it
// is datable and not an extra, whether it carries a backdrop, and the two keys
// the restriction predicate is a function of. Summing the buckets that pass a
// viewer's restriction gives the same number the count gave, in O(buckets).
//
// The exactness argument is the same one the browse buckets make: every item in
// a bucket carries the same content ratings and the same labels, so the
// restriction either admits all of them or none.

// homeBucket is one counted group of items.
type homeBucket struct {
	available, recent, backdrop bool
	ratingKey, labelKey         string
	total                       int
}

// homeBucketTotal sums the buckets of one library that match a filter over the
// three flags and pass the viewer's restriction.
func (s *Service) homeBucketTotal(library, kind string, available, recent, backdrop int, r identity.ContentRestrictions) (int, error) {
	where := `library_id=(SELECT id FROM catalog_libraries WHERE library_id=?)`
	args := []any{library}
	if kind != "" {
		value, err := compactcatalog.ParseKind(kind)
		if err != nil {
			return 0, err
		}
		where += ` AND kind=?`
		args = append(args, value)
	}
	for column, want := range map[string]int{"available": available, "recent": recent, "backdrop": backdrop} {
		if want >= 0 {
			where += ` AND ` + column + `=?`
			args = append(args, want)
		}
	}
	rows, err := s.read().Query(`SELECT available,recent,backdrop,rating_key,label_key,total FROM catalog_home_buckets WHERE `+where+` AND total>0`, args...)
	if err != nil {
		return 0, err
	}
	buckets := []homeBucket{}
	for rows.Next() {
		var bucket homeBucket
		if err = rows.Scan(&bucket.available, &bucket.recent, &bucket.backdrop, &bucket.ratingKey, &bucket.labelKey, &bucket.total); err != nil {
			rows.Close()
			return 0, err
		}
		buckets = append(buckets, bucket)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if !r.Active() {
		total := 0
		for _, bucket := range buckets {
			total += bucket.total
		}
		return total, nil
	}
	ages, err := s.ratingAges()
	if err != nil {
		return 0, err
	}
	blocked := map[string]bool{}
	for _, label := range r.BlockedLabels {
		blocked[strings.ToLower(label)] = true
	}
	total := 0
	for _, bucket := range buckets {
		if bucketVisible(browseBucket{ratingKey: bucket.ratingKey, labelKey: bucket.labelKey}, ages, r, blocked) {
			total += bucket.total
		}
	}
	return total, nil
}

// homeRecentTotal is the `recent_<library>` row's total: every visible item in
// the library that carries an added date and is not an extra, under the viewer's
// restriction.
func (s *Service) homeRecentTotal(library string, r identity.ContentRestrictions) (int, error) {
	return s.homeBucketTotal(library, "", 1, 1, -1, r)
}
