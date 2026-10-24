package claimweb

// LookupPending adds a verified display fact only to authenticated lookup.
// Pending registration and the original signed claim retain their wire shape.
type LookupPending struct {
	Pending
	OtherPeopleCount *int `json:"otherPeopleCount,omitempty"`
}
