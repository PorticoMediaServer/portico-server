package reading

// RevisionError reports that a continuation no longer matches the authorized
// ready image. Catalog and Viewer are the current content revision of that
// same snapshot, so a caller can answer with exactly the revision a fresh read
// of this library and viewer publishes.
//
// It is still ErrChanged: authorization already succeeded before a continuation
// is refused, so this carries no fact an authorized read would not return. A
// refused authority never produces one.
type RevisionError struct{ Catalog, Viewer int64 }

func (e *RevisionError) Error() string { return ErrChanged.Error() }
func (e *RevisionError) Unwrap() error { return ErrChanged }
