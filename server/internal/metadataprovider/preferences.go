package metadataprovider

import "context"

type screenPreferencesKey struct{}
type screenPreferences struct{ fallback, anime string }

// WithScreenPreferences carries one revisioned request's presentation choices.
// Shared provider instances and their caches never retain a caller's settings.
func WithScreenPreferences(ctx context.Context, fallback, anime string) context.Context {
	return context.WithValue(ctx, screenPreferencesKey{}, screenPreferences{fallback, anime})
}
func preferences(ctx context.Context) screenPreferences {
	if p, ok := ctx.Value(screenPreferencesKey{}).(screenPreferences); ok {
		return p
	}
	return screenPreferences{"", "language"}
}
func animeLanguage(ctx context.Context, locale string) string {
	switch preferences(ctx).anime {
	case "english":
		return "en"
	case "native":
		return "ja"
	case "romaji":
		return "romaji"
	}
	return locale
}
