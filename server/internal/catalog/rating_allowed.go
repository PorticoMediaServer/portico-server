package catalog

import (
	"strings"

	"portico.local/server/internal/access"
	"portico.local/server/internal/identity"
)

// RatingAllowed is the catalogue restriction predicate (itemRestrictionClause)
// for one thing that carries at most a content rating and no labels, such as
// a live-source guide programme (Channels spec §8.1): the profile's age
// ceiling, its unrated rule, and the member's rating ladder. An empty or
// unknown rating is unrated.
func RatingAllowed(r identity.ContentRestrictions, rating string) bool {
	age, rated := identity.RatingAge(rating)
	if r.MaximumAge != nil && rated && age > *r.MaximumAge {
		return false
	}
	if r.BlockUnrated && !rated {
		return false
	}
	if r.MemberMaxRating != "" {
		value := strings.ToUpper(strings.TrimSpace(rating))
		ceiling, index := -1, -1
		for i, v := range access.ContentRatings {
			if v == strings.ToUpper(strings.TrimSpace(r.MemberMaxRating)) {
				ceiling = i
			}
			if v == value {
				index = i
			}
		}
		if index < 0 {
			return r.MemberAllowUnrated
		}
		if ceiling < 0 || index > ceiling {
			return false
		}
	}
	return true
}
