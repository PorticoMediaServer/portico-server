package directory

func endingSequenceSchema() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991}
}
func endingTimeSchema() map[string]any {
	return map[string]any{"type": "string", "format": "date-time"}
}
func endingCauseSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"owner_removed"}}
}
func (MembershipEnding) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"required":   []string{"sequence", "cause", "endedAt", "expiresAt"},
		"properties": map[string]any{"sequence": endingSequenceSchema(), "cause": endingCauseSchema(), "endedAt": endingTimeSchema(), "expiresAt": endingTimeSchema()}}
}
func (MembershipEndingNotice) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"serverId", "membershipId", "sequence", "cause", "expiresAt"},
		"properties": map[string]any{
			"serverId":     map[string]any{"type": "string", "pattern": `^srv_[A-Za-z0-9_-]{43}$`},
			"membershipId": map[string]any{"type": "string", "pattern": `^[0-9a-f]{32}$`},
			"sequence":     endingSequenceSchema(), "cause": endingCauseSchema(), "expiresAt": endingTimeSchema(),
			"serverName": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048},
		}}
}
