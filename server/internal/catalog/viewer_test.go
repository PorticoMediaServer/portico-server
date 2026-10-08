package catalog

// viewerForItem keeps the older test fixtures focused on their assertions
// while exercising the same explicit authority contract as HTTP callers.
func viewerForItem(s *Service, profile, fence, item string) Viewer {
	library, _ := s.LibraryForItem(item)
	return Viewer{Profile: profile, Fence: fence, Libraries: []string{library}}
}
