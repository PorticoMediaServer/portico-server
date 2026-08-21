// Package browsecontract owns Portico's canonical V1 browse vocabulary and
// query-shape validation. Product-contract projection, query compilation, and
// server-owned consumers such as Library Channels must all use this package
// rather than maintaining parallel field or operator lists.
package browsecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	MaximumDepth   = 5
	MaximumClauses = 40
	MaximumBytes   = 64 * 1024
)

type ValueType string

const (
	ValueString      ValueType = "string"
	ValueEnum        ValueType = "enum"
	ValueNumber      ValueType = "number"
	ValueDuration    ValueType = "duration"
	ValueDate        ValueType = "date"
	ValueBoolean     ValueType = "boolean"
	ValueIdentitySet ValueType = "identity-set"
	ValuePresence    ValueType = "presence"
)

type Field struct {
	ID            string
	ValueType     ValueType
	Operators     []string
	AllowedValues []string
}

var fields = []Field{
	{ID: "entityKind", ValueType: ValueEnum, Operators: []string{"equals", "not-equals", "in", "not-in"}, AllowedValues: []string{"movie", "show", "season", "episode", "special", "artist", "album", "track", "author", "book", "audiobook-series", "chapter", "collection", "playlist", "category", "recording", "live-channel", "live-program", "person", "extra"}},
	{ID: "title", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "alternateTitle", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "originalTitle", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "language", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "status", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "releaseDate", ValueType: ValueDate, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}},
	{ID: "year", ValueType: ValueNumber, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between"}},
	{ID: "decade", ValueType: ValueNumber, Operators: []string{"equals"}},
	{ID: "dateAdded", ValueType: ValueDate, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between"}},
	{ID: "playState", ValueType: ValueEnum, Operators: []string{"equals", "not-equals", "in", "not-in"}, AllowedValues: []string{"unplayed", "in-progress", "played"}},
	{ID: "favorite", ValueType: ValueBoolean, Operators: []string{"is"}},
	{ID: "watchlisted", ValueType: ValueBoolean, Operators: []string{"is"}},
	{ID: "personalRating", ValueType: ValueNumber, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}},
	{ID: "genre", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "tag", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "label", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "author", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "narrator", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "series", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "studio", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "company", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "network", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "country", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "keyword", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "collection", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "franchise", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "actor", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "director", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "writer", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "creator", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "credit", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "contentRating", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "regionalCertification", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "communityRating", ValueType: ValueNumber, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}},
	{ID: "criticRating", ValueType: ValueNumber, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}},
	{ID: "audienceRating", ValueType: ValueNumber, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}},
	{ID: "acceptedProviderIdentity", ValueType: ValueIdentitySet, Operators: []string{"contains", "not-contains", "contains-any", "contains-all"}},
	{ID: "availability", ValueType: ValueEnum, Operators: []string{"equals", "not-equals", "in", "not-in"}, AllowedValues: []string{"available", "partial", "unavailable"}},
	{ID: "resolution", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "dynamicRange", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "source", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "mediaVersion", ValueType: ValueString, Operators: []string{"equals", "not-equals", "contains", "not-contains", "starts-with"}},
	{ID: "durationSeconds", ValueType: ValueDuration, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between"}},
	{ID: "lastPlayedAt", ValueType: ValueDate, Operators: []string{"equals", "less-than", "at-most", "greater-than", "at-least", "between", "is-present", "is-missing"}},
}

func Fields() []Field {
	result := make([]Field, len(fields))
	for index, field := range fields {
		result[index] = field
		result[index].Operators = append([]string(nil), field.Operators...)
		result[index].AllowedValues = append([]string(nil), field.AllowedValues...)
	}
	return result
}

func FieldByID(id string) (Field, bool) {
	for _, field := range fields {
		if field.ID == id {
			return field, true
		}
	}
	return Field{}, false
}

type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

type ValidationOptions struct {
	AllowedFields map[string]struct{}
	AllowEmpty    bool
}

func ValidateQuery(raw json.RawMessage, options ValidationOptions) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > MaximumBytes {
		return issue("query", "must not exceed 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return issue("query", "must be valid JSON")
	}
	if decoder.Decode(&struct{}{}) == nil {
		return issue("query", "must contain exactly one JSON value")
	}
	clauses := 0
	var inspect func(any, int, string) error
	inspect = func(current any, depth int, path string) error {
		if depth > MaximumDepth {
			return issue(path, fmt.Sprintf("must not exceed %d levels", MaximumDepth))
		}
		object, ok := current.(map[string]any)
		if !ok {
			return issue(path, "must be a query object")
		}
		if len(object) == 0 {
			if depth == 0 && options.AllowEmpty {
				return nil
			}
			return issue(path, "must not be empty")
		}
		if _, leaf := object["field"]; leaf {
			if len(object) != 3 {
				return issue(path, "filter nodes must contain exactly field, operator, and value")
			}
			fieldID, fieldOK := object["field"].(string)
			operator, operatorOK := object["operator"].(string)
			if !fieldOK || !operatorOK || fieldID != strings.TrimSpace(fieldID) || operator != strings.TrimSpace(operator) {
				return issue(path, "field and operator must be canonical strings")
			}
			field, exists := FieldByID(fieldID)
			if !exists {
				return issue(path+".field", fmt.Sprintf("field %q is not supported", fieldID))
			}
			if options.AllowedFields != nil {
				if _, allowed := options.AllowedFields[fieldID]; !allowed {
					return issue(path+".field", fmt.Sprintf("field %q is not schedule-safe", fieldID))
				}
			}
			if !contains(field.Operators, operator) {
				return issue(path+".operator", fmt.Sprintf("operator %q is not valid for %s", operator, fieldID))
			}
			clauses++
			if clauses > MaximumClauses {
				return issue(path, fmt.Sprintf("must not exceed %d predicates", MaximumClauses))
			}
			return validateValue(field, operator, object["value"], path+".value")
		}
		if len(object) != 1 {
			return issue(path, "group nodes must contain exactly one of all, any, or not")
		}
		for operator, child := range object {
			switch operator {
			case "all", "any":
				children, ok := child.([]any)
				if !ok || len(children) == 0 || len(children) > MaximumClauses {
					return issue(path+"."+operator, fmt.Sprintf("must contain between 1 and %d query nodes", MaximumClauses))
				}
				for index, nested := range children {
					if err := inspect(nested, depth+1, fmt.Sprintf("%s.%s[%d]", path, operator, index)); err != nil {
						return err
					}
				}
			case "not":
				return inspect(child, depth+1, path+".not")
			default:
				return issue(path, fmt.Sprintf("unknown query node %q", operator))
			}
		}
		return nil
	}
	return inspect(value, 0, "query")
}

func validateValue(field Field, operator string, value any, path string) error {
	if operator == "is-present" || operator == "is-missing" {
		if value != nil {
			return issue(path, "must be null for a presence operator")
		}
		return nil
	}
	if field.ValueType == ValueBoolean {
		if _, ok := value.(bool); !ok {
			return issue(path, "must be true or false")
		}
		return nil
	}
	arrayRequired := operator == "between" || operator == "contains-any" || operator == "contains-all" || operator == "in" || operator == "not-in"
	values, isArray := value.([]any)
	if arrayRequired != isArray {
		if arrayRequired {
			return issue(path, "must be an array for this operator")
		}
		return issue(path, "must be a scalar for this operator")
	}
	if isArray {
		if len(values) == 0 || len(values) > 100 || (operator == "between" && len(values) != 2) {
			return issue(path, "contains an invalid number of values")
		}
		for _, candidate := range values {
			if err := validateScalar(field, candidate, path); err != nil {
				return err
			}
		}
		return nil
	}
	return validateScalar(field, value, path)
}

func validateScalar(field Field, value any, path string) error {
	switch field.ValueType {
	case ValueString, ValueEnum, ValueIdentitySet, ValueDate:
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" || len(text) > 500 {
			return issue(path, "must be a non-empty string no longer than 500 bytes")
		}
		if field.ValueType == ValueDate && !validDate(text) {
			return issue(path, "must use YYYY-MM-DD or RFC 3339")
		}
		if len(field.AllowedValues) > 0 && !contains(field.AllowedValues, text) {
			return issue(path, "contains an unsupported value")
		}
		return nil
	case ValueNumber, ValueDuration:
		number, ok := value.(json.Number)
		if !ok {
			return issue(path, "must be numeric")
		}
		parsed, err := number.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return issue(path, "must be a finite number")
		}
		return nil
	default:
		return issue(path, "has an unsupported value type")
	}
}

func validDate(value string) bool {
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func issue(path, message string) error { return &ValidationError{Path: path, Message: message} }
