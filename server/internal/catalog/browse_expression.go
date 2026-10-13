package catalog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"portico.local/server/internal/facettext"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/sorttext"
	"time"
)

// BrowseNode is one node of the browse expression tree: a group (all/any/not) or
// a predicate (field/operator/value). Parsing and validation happen together so
// every rejection names the exact path that failed.
type BrowseNode struct {
	All      []BrowseNode `json:"all,omitempty"`
	Any      []BrowseNode `json:"any,omitempty"`
	Not      *BrowseNode  `json:"not,omitempty"`
	Field    string       `json:"field,omitempty"`
	Operator string       `json:"operator,omitempty"`
	Value    any          `json:"value,omitempty"`
}

// MarshalJSON writes a predicate as exactly {field, operator, value}, with
// `"value": null` for the presence operators, and a group as exactly its one
// key (CD-29). The published schema says value is null for is-present and
// is-missing, and clients read a predicate as those three keys; the default
// encoding dropped a null value, so a saved view or deep link echoed back with
// a presence filter was refused by the client that made it.
func (n BrowseNode) MarshalJSON() ([]byte, error) {
	if n.Field != "" || n.Operator != "" {
		return json.Marshal(struct {
			Field    string `json:"field"`
			Operator string `json:"operator"`
			Value    any    `json:"value"`
		}{n.Field, n.Operator, n.Value})
	}
	type group struct {
		All []BrowseNode `json:"all,omitempty"`
		Any []BrowseNode `json:"any,omitempty"`
		Not *BrowseNode  `json:"not,omitempty"`
	}
	return json.Marshal(group{n.All, n.Any, n.Not})
}

// BrowseValidationError names the offending path so a client can attach the
// message to the control that produced it.
type BrowseValidationError struct {
	Path    string
	Message string
}

func (e *BrowseValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func browseIssue(path, message string) error {
	return &BrowseValidationError{Path: path, Message: message}
}

// ParseBrowseQuery validates raw JSON against the published vocabulary and
// returns the typed tree. A nil result means "no filter".
func ParseBrowseQuery(raw json.RawMessage, path string) (*BrowseNode, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	if len(raw) > BrowseMaximumBytes {
		return nil, browseIssue(path, "query must not exceed 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, browseIssue(path, "query must be valid JSON")
	}
	if decoder.Decode(new(any)) == nil {
		return nil, browseIssue(path, "query must contain exactly one JSON value")
	}
	clauses := 0
	return parseBrowseNode(value, 0, path, &clauses)
}

func parseBrowseNode(value any, depth int, path string, clauses *int) (*BrowseNode, error) {
	if depth > BrowseMaximumDepth {
		return nil, browseIssue(path, fmt.Sprintf("query must not nest more than %d levels", BrowseMaximumDepth))
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, browseIssue(path, "must be a query object")
	}
	if len(object) == 0 {
		if depth == 0 {
			return nil, nil
		}
		return nil, browseIssue(path, "must not be empty")
	}
	if _, leaf := object["field"]; leaf {
		return parseBrowsePredicate(object, path, clauses)
	}
	if len(object) != 1 {
		return nil, browseIssue(path, "group nodes must contain exactly one of all, any or not")
	}
	for operator, child := range object {
		switch operator {
		case "all", "any":
			children, ok := child.([]any)
			if !ok || len(children) == 0 || len(children) > BrowseMaximumClauses {
				return nil, browseIssue(path+"."+operator, fmt.Sprintf("must contain between 1 and %d query nodes", BrowseMaximumClauses))
			}
			parsed := make([]BrowseNode, 0, len(children))
			for index, nested := range children {
				node, err := parseBrowseNode(nested, depth+1, fmt.Sprintf("%s.%s[%d]", path, operator, index), clauses)
				if err != nil {
					return nil, err
				}
				if node == nil {
					return nil, browseIssue(fmt.Sprintf("%s.%s[%d]", path, operator, index), "must not be empty")
				}
				parsed = append(parsed, *node)
			}
			if operator == "all" {
				return &BrowseNode{All: parsed}, nil
			}
			return &BrowseNode{Any: parsed}, nil
		case "not":
			node, err := parseBrowseNode(child, depth+1, path+".not", clauses)
			if err != nil {
				return nil, err
			}
			if node == nil {
				return nil, browseIssue(path+".not", "must not be empty")
			}
			return &BrowseNode{Not: node}, nil
		default:
			return nil, browseIssue(path, fmt.Sprintf("unknown query node %q", operator))
		}
	}
	return nil, browseIssue(path, "must be a query object")
}

func parseBrowsePredicate(object map[string]any, path string, clauses *int) (*BrowseNode, error) {
	if len(object) != 3 {
		return nil, browseIssue(path, "predicate nodes must contain exactly field, operator and value")
	}
	id, idOK := object["field"].(string)
	operator, operatorOK := object["operator"].(string)
	if !idOK || !operatorOK {
		return nil, browseIssue(path, "field and operator must be strings")
	}
	field, exists := browseFieldByID(id)
	if !exists {
		return nil, browseIssue(path+".field", fmt.Sprintf("field %q is not supported", id))
	}
	if !containsString(field.Operators, operator) {
		return nil, browseIssue(path+".operator", fmt.Sprintf("operator %q is not valid for %s", operator, id))
	}
	*clauses++
	if *clauses > BrowseMaximumClauses {
		return nil, browseIssue(path, fmt.Sprintf("query must not exceed %d predicates", BrowseMaximumClauses))
	}
	value, err := validateBrowseValue(field, operator, object["value"], path+".value")
	if err != nil {
		return nil, err
	}
	return &BrowseNode{Field: id, Operator: operator, Value: value}, nil
}

func validateBrowseValue(field browseField, operator string, value any, path string) (any, error) {
	if operator == "is-present" || operator == "is-missing" {
		if value != nil {
			return nil, browseIssue(path, "must be null for a presence operator")
		}
		return nil, nil
	}
	if field.Type == valueBoolean {
		flag, ok := value.(bool)
		if !ok {
			return nil, browseIssue(path, "must be true or false")
		}
		return flag, nil
	}
	list := operator == "between" || operator == "in" || operator == "not-in" || operator == "contains-any" || operator == "contains-all"
	values, isList := value.([]any)
	if list != isList {
		if list {
			return nil, browseIssue(path, "must be an array for this operator")
		}
		return nil, browseIssue(path, "must be a scalar for this operator")
	}
	if !isList {
		return validateBrowseScalar(field, value, path)
	}
	if len(values) == 0 || len(values) > BrowseMaximumValues || (operator == "between" && len(values) != 2) {
		return nil, browseIssue(path, "contains an invalid number of values")
	}
	out := make([]any, 0, len(values))
	for index, candidate := range values {
		scalar, err := validateBrowseScalar(field, candidate, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return nil, err
		}
		out = append(out, scalar)
	}
	return out, nil
}

func validateBrowseScalar(field browseField, value any, path string) (any, error) {
	switch field.Type {
	case valueString, valueEnum, valueSet, valueDate:
		text, ok := value.(string)
		if !ok {
			text, ok = numberText(value, field)
		}
		if !ok || strings.TrimSpace(text) == "" || len(text) > 500 || strings.ContainsAny(text, "\x00\r\n") {
			return nil, browseIssue(path, "must be a non-empty string no longer than 500 bytes")
		}
		if field.Type == valueDate && !validBrowseDate(text) {
			return nil, browseIssue(path, "must use YYYY-MM-DD or RFC 3339")
		}
		if len(field.AllowedValues) > 0 && !containsString(field.AllowedValues, text) {
			return nil, browseIssue(path, "contains an unsupported value")
		}
		return text, nil
	case valueNumber:
		number, ok := value.(json.Number)
		if !ok {
			// CD-02: facet values are published as strings (a decade facet
			// answers "1990"), and a client filters with the values it was
			// given, so numeric text is accepted wherever a number is.
			if text, isText := value.(string); isText {
				if _, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err != nil {
					return nil, browseIssue(path, "must be numeric")
				}
				number = json.Number(strings.TrimSpace(text))
			} else if plain, isFloat := value.(float64); isFloat {
				number = json.Number(strconv.FormatFloat(plain, 'f', -1, 64))
			} else if whole, isInt := value.(int); isInt {
				number = json.Number(strconv.Itoa(whole))
			} else {
				return nil, browseIssue(path, "must be numeric")
			}
		}
		parsed, err := number.Float64()
		if field.ID == "durationMs" && (parsed < 0 || parsed > 9007199254740991 || math.Trunc(parsed) != parsed) {
			return nil, browseIssue(path, "duration must be whole milliseconds")
		}
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, browseIssue(path, "must be a finite number")
		}
		return parsed, nil
	}
	return nil, browseIssue(path, "has an unsupported value type")
}

func numberText(value any, field browseField) (string, bool) {
	if field.Type != valueEnum && field.Type != valueSet && field.Type != valueString {
		return "", false
	}
	if number, ok := value.(json.Number); ok {
		return number.String(), true
	}
	return "", false
}

func validBrowseDate(value string) bool {
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return true
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ---- compilation ----

// browseCompiler turns the validated tree into one parameterised WHERE clause
// over catalog_browse_rows aliased `e`. Every predicate is either a scalar over
// the entity row or an EXISTS over the entity's member items, so the join
// surface stays bounded no matter how deep the expression is.
type browseCompiler struct {
	restrictions identity.ContentRestrictions
	profile      string
	args         []any
}

func (c *browseCompiler) compile(node *BrowseNode) (string, error) {
	if node == nil {
		return "1=1", nil
	}
	switch {
	case node.All != nil:
		return c.group(node.All, " AND ")
	case node.Any != nil:
		return c.group(node.Any, " OR ")
	case node.Not != nil:
		inner, err := c.compile(node.Not)
		if err != nil {
			return "", err
		}
		return "NOT(" + inner + ")", nil
	case node.Field != "":
		return c.predicate(*node)
	}
	return "1=1", nil
}

func (c *browseCompiler) group(children []BrowseNode, glue string) (string, error) {
	parts := make([]string, 0, len(children))
	for index := range children {
		part, err := c.compile(&children[index])
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	return "(" + strings.Join(parts, glue) + ")", nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func (c *browseCompiler) predicate(node BrowseNode) (string, error) {
	field, ok := browseFieldByID(node.Field)
	if !ok {
		return "", browseIssue("query.field", "field is not supported")
	}
	if field.ID == "durationMs" {
		if value, ok := node.Value.(float64); ok {
			node.Value = value / 1000
		}
		if values, ok := node.Value.([]any); ok {
			converted := make([]any, len(values))
			for i, v := range values {
				if number, ok := v.(float64); ok {
					converted[i] = number / 1000
				} else {
					converted[i] = v
				}
			}
			node.Value = converted
		}
	}
	if field.Entity != "" {
		return c.entityPredicate(field, node)
	}
	return c.itemPredicate(field, node)
}

func foldBrowseText(field browseField) bool {
	switch field.ID {
	case "collection", "memberOf", "person", "list":
		return false
	}
	return field.Type == valueString || field.Type == valueSet || field.Type == valueEnum
}
func collated(field browseField, expression string) string {
	if foldBrowseText(field) {
		return "portico_filter_fold(" + expression + ")"
	}
	if field.ID == "collection" || field.ID == "memberOf" || field.ID == "person" || field.ID == "list" {
		return "(" + expression + ")"
	}
	if field.Type == valueString || field.Type == valueEnum || field.Type == valueSet {
		return "(" + expression + ") COLLATE NOCASE"
	}
	return "(" + expression + ")"
}

func likeValue(value any, prefixOnly bool) string {
	text, _ := value.(string)
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, "%", `\%`)
	text = strings.ReplaceAll(text, "_", `\_`)
	if prefixOnly {
		return text + "%"
	}
	return "%" + text + "%"
}

// browseDatePeriod is the period a date operand names: the whole UTC day for
// YYYY-MM-DD, and for an RFC 3339 instant of any offset the unit it was
// written to, a second or the last fraction digit given.
func browseDatePeriod(value any) (from, until time.Time, ok bool) {
	text, _ := value.(string)
	if day, err := time.Parse("2006-01-02", text); err == nil {
		return day, day.AddDate(0, 0, 1), true
	}
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return from, until, false
	}
	unit := time.Second
	if match := browseDateFraction.FindStringSubmatch(text); match != nil {
		for range min(len(match[1]), 9) {
			unit /= 10
		}
	}
	at = at.UTC().Truncate(unit)
	return at, at.Add(unit), true
}

var browseDateFraction = regexp.MustCompile(`:[0-9]{2}\.([0-9]+)`)

// browseDateBound is one end of a period as stored text sorts against it.
// Stored dates are UTC text: a date, or a timestamp whose fraction has any
// number of digits or none. Text order is time order for all of them except
// the timestamps of the bound's own second whose fraction stops short of the
// bound's ("…:11Z" and "…:11.5Z" against "…:11.501"): they sort after it and
// are earlier, so they are named one by one in short.
type browseDateBound struct {
	text  string
	short []any
}

func newBrowseDateBound(at time.Time) browseDateBound {
	at = at.UTC()
	second := at.Truncate(time.Second)
	if at.Equal(second) {
		// A stored date is its day's midnight.
		if h, m, s := at.Clock(); h == 0 && m == 0 && s == 0 {
			return browseDateBound{text: at.Format("2006-01-02")}
		}
		return browseDateBound{text: at.Format("2006-01-02T15:04:05")}
	}
	base := second.Format("2006-01-02T15:04:05")
	fraction := strings.TrimRight(fmt.Sprintf("%09d", at.Nanosecond()), "0")
	bound := browseDateBound{text: base + "." + fraction, short: []any{base + "Z"}}
	for digits := 1; digits < len(fraction); digits++ {
		bound.short = append(bound.short, base+"."+fraction[:digits]+"Z")
	}
	return bound
}

// before renders "the stored date is earlier than this bound"; from renders
// "it is this bound or later". Both are unknown, so false, for a missing date.
func (c *browseCompiler) before(target string, bound browseDateBound) string {
	c.args = append(c.args, bound.text)
	if len(bound.short) == 0 {
		return target + "<?"
	}
	c.args = append(c.args, bound.short...)
	return "(" + target + "<? OR " + target + " IN (" + placeholders(len(bound.short)) + "))"
}

func (c *browseCompiler) from(target string, bound browseDateBound) string {
	c.args = append(c.args, bound.text)
	if len(bound.short) == 0 {
		return target + ">=?"
	}
	c.args = append(c.args, bound.short...)
	return "(" + target + ">=? AND " + target + " NOT IN (" + placeholders(len(bound.short)) + "))"
}

// dateComparison renders an operator over a date field: equality is falling in
// the operand's period, and an inclusive bound includes all of it.
func (c *browseCompiler) dateComparison(expression, operator string, value any) (string, bool, error) {
	target := "(" + expression + ")"
	if operator == "between" {
		values, _ := value.([]any)
		if len(values) != 2 {
			return "", true, browseIssue("query.value", "between requires exactly two values")
		}
		first, _, firstOK := browseDatePeriod(values[0])
		_, last, lastOK := browseDatePeriod(values[1])
		if !firstOK || !lastOK {
			return "", true, browseIssue("query.value", "must use YYYY-MM-DD or RFC 3339")
		}
		return "(" + c.from(target, newBrowseDateBound(first)) + " AND " + c.before(target, newBrowseDateBound(last)) + ")", true, nil
	}
	start, end, ok := browseDatePeriod(value)
	if !ok {
		return "", false, nil
	}
	from, until := newBrowseDateBound(start), newBrowseDateBound(end)
	switch operator {
	case "equals":
		return "(" + c.from(target, from) + " AND " + c.before(target, until) + ")", true, nil
	case "not-equals":
		return "(" + target + " IS NULL OR " + c.before(target, from) + " OR " + c.from(target, until) + ")", true, nil
	case "less-than":
		return c.before(target, from), true, nil
	case "at-most":
		return c.before(target, until), true, nil
	case "greater-than":
		return c.from(target, until), true, nil
	case "at-least":
		return c.from(target, from), true, nil
	}
	return "", false, nil
}

// comparison renders one operator against a value expression and binds its args.
func (c *browseCompiler) comparison(field browseField, expression, operator string, value any) (string, error) {
	if field.Type == valueDate {
		if clause, handled, err := c.dateComparison(expression, operator, value); handled {
			return clause, err
		}
	}
	target := collated(field, expression)
	if foldBrowseText(field) {
		fold := func(v any) any {
			if text, ok := v.(string); ok {
				return sorttext.Fold(text)
			}
			return v
		}
		if values, ok := value.([]any); ok {
			copy := make([]any, len(values))
			for i, v := range values {
				copy[i] = fold(v)
			}
			value = copy
		} else {
			value = fold(value)
		}
	}
	switch operator {
	case "equals":
		c.args = append(c.args, value)
		return target + "=?", nil
	case "not-equals":
		c.args = append(c.args, value)
		return "((" + expression + ") IS NULL OR " + target + "<>?)", nil
	case "contains":
		if field.Type == valueSet {
			c.args = append(c.args, value)
			return target + "=?", nil
		}
		c.args = append(c.args, likeValue(value, false))
		return target + " LIKE ? ESCAPE '\\'", nil
	case "starts-with":
		c.args = append(c.args, likeValue(value, true))
		return target + " LIKE ? ESCAPE '\\'", nil
	case "less-than", "at-most", "greater-than", "at-least":
		c.args = append(c.args, value)
		return target + map[string]string{"less-than": "<?", "at-most": "<=?", "greater-than": ">?", "at-least": ">=?"}[operator], nil
	case "between":
		values, _ := value.([]any)
		if len(values) != 2 {
			return "", browseIssue("query.value", "between requires exactly two values")
		}
		c.args = append(c.args, values[0], values[1])
		return target + " BETWEEN ? AND ?", nil
	case "in", "contains-any":
		values, _ := value.([]any)
		c.args = append(c.args, values...)
		return target + " IN (" + placeholders(len(values)) + ")", nil
	case "not-in":
		values, _ := value.([]any)
		c.args = append(c.args, values...)
		return "((" + expression + ") IS NULL OR " + target + " NOT IN (" + placeholders(len(values)) + "))", nil
	case "is-present":
		return "(" + expression + ") IS NOT NULL", nil
	case "is-missing":
		return "(" + expression + ") IS NULL", nil
	}
	return "", browseIssue("query.operator", "operator is not supported")
}

func (c *browseCompiler) entityPredicate(field browseField, node BrowseNode) (string, error) {
	if field.ID == "search" {
		q, ok := node.Value.(string)
		if !ok {
			return "", browseIssue("query.value", "search requires text")
		}
		normalized, match, err := NormalizeSearch(q)
		if err != nil {
			return "", browseIssue("query.value", "search requires between 1 and 128 characters")
		}
		c.args = append(c.args, match)
		clause := `e.entity_id IN(SELECT d.entity_id FROM catalog_search_titles JOIN catalog_search_documents d ON d.entity_id=catalog_search_titles.rowid WHERE catalog_search_titles MATCH ?`
		for _, word := range strings.Fields(normalized) {
			if strings.ContainsFunc(word, facettext.CJK) {
				clause += ` AND instr(d.normalized,?)>0`
				c.args = append(c.args, word)
			}
		}
		return clause + ")", nil
	}

	return c.comparison(field, field.Entity, node.Operator, node.Value)
}

// exists wraps a member-item condition. Profile placeholders inside the join are
// bound before the condition's own arguments so ordinal positions line up.
func (c *browseCompiler) exists(field browseField, condition func() (string, error)) (string, error) {
	prefix := "EXISTS(SELECT 1 FROM catalog_browse_memberships bei " + field.Join + " WHERE bei.entity_id=e.entity_id AND "
	if field.ID == "list" {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(c.profile, "viewer:"))
		var owner []string
		if err != nil || !strings.HasPrefix(c.profile, "viewer:") || json.Unmarshal(raw, &owner) != nil || len(owner) != 3 {
			return "0", nil
		}
		for i := 0; i < 2; i++ {
			for _, part := range owner {
				c.args = append(c.args, part)
			}
		}
	} else {
		for i := 0; i < field.Profile; i++ {
			c.args = append(c.args, c.profile)
		}
	}
	restriction, args := ItemRestrictionSQL("bei.item_id", c.restrictions)
	prefix += "(" + restriction + ") AND "
	c.args = append(c.args, args...)
	body, err := condition()
	if err != nil {
		return "", err
	}
	return prefix + body + ")", nil
}

func (c *browseCompiler) itemPredicate(field browseField, node BrowseNode) (string, error) {
	if field.ID == "favorite" || field.ID == "watchlisted" || field.ID == "personalRating" || field.ID == "lastPlayedAt" {
		return c.personalTitlePredicate(field, node)
	}
	if field.ID == "subtitleLanguage" {
		return c.subtitleLanguagePredicate(field, node)
	}
	if field.Type == valueBoolean {
		flag, _ := node.Value.(bool)
		positive := (node.Operator == "equals") == flag
		clause, err := c.exists(field, func() (string, error) { return "(" + field.Value + ")=1", nil })
		if err != nil {
			return "", err
		}
		if positive {
			return clause, nil
		}
		return "NOT " + clause, nil
	}
	switch node.Operator {
	case "not-equals", "not-in", "is-missing":
		positive := map[string]string{"not-equals": "equals", "not-in": "in", "is-missing": "is-present"}[node.Operator]
		clause, err := c.exists(field, func() (string, error) {
			return c.comparison(field, field.Value, positive, node.Value)
		})
		if err != nil {
			return "", err
		}
		return "NOT " + clause, nil
	case "contains-all":
		values, _ := node.Value.([]any)
		parts := make([]string, 0, len(values))
		for _, value := range values {
			single := value
			clause, err := c.exists(field, func() (string, error) {
				return c.comparison(field, field.Value, "equals", single)
			})
			if err != nil {
				return "", err
			}
			parts = append(parts, clause)
		}
		return "(" + strings.Join(parts, " AND ") + ")", nil
	}
	return c.exists(field, func() (string, error) {
		return c.comparison(field, field.Value, node.Operator, node.Value)
	})
}

// ---- non-personal surfaces (Library Channels) ----

// ErrBrowsePersonalField is returned when a shared surface (a Library Channel
// rule) names a field whose value depends on one viewer's profile.
var ErrBrowsePersonalField = browseIssue("query", "watched, favorite, watchlist, personal rating and last played depend on one profile and can't select a shared channel")

// CompileBrowseSelection compiles a validated tree into a WHERE clause over
// catalog_browse_rows aliased `e`, for shared surfaces outside the browse routes
// (Library Channels). Personal fields are refused: a shared surface has no
// single viewer. Additive: the browse routes do not use it.
func CompileBrowseSelection(node *BrowseNode) (string, []any, error) {
	if BrowseNodePersonal(node) {
		return "", nil, ErrBrowsePersonalField
	}
	c := &browseCompiler{}
	where, err := c.compile(node)
	if err != nil {
		return "", nil, err
	}
	return where, c.args, nil
}

// CompileCompactBrowseSelection shares the validated non-personal vocabulary
// with Library Channels over catalog_browse_rows aliased e.
func CompileCompactBrowseSelection(node *BrowseNode) (string, []any, error) {
	if BrowseNodePersonal(node) {
		return "", nil, ErrBrowsePersonalField
	}
	c := &browseCompiler{}
	where, err := c.compile(node)
	if err != nil {
		return "", nil, err
	}
	return where, c.args, nil
}

// BrowseNodePersonal reports whether any predicate reads a profile-bound field.
func BrowseNodePersonal(node *BrowseNode) bool {
	if node == nil {
		return false
	}
	for i := range node.All {
		if BrowseNodePersonal(&node.All[i]) {
			return true
		}
	}
	for i := range node.Any {
		if BrowseNodePersonal(&node.Any[i]) {
			return true
		}
	}
	if BrowseNodePersonal(node.Not) {
		return true
	}
	if node.Field != "" {
		field, ok := browseFieldByID(node.Field)
		return ok && field.Profile > 0
	}
	return false
}

// JSONSchema declares the expression's wire fields explicitly. The parser
// enforces the union, recursive children and published depth/clause bounds.
func (BrowseNode) JSONSchema() map[string]any {
	child := map[string]any{"type": "object", "required": []string{}, "properties": map[string]any{}, "additionalProperties": map[string]any{}}
	group := map[string]any{"type": "array", "minItems": 1, "maxItems": BrowseMaximumClauses, "items": child}
	return map[string]any{"type": "object", "required": []string{}, "additionalProperties": false, "properties": map[string]any{
		"field": map[string]any{"type": "string"}, "operator": map[string]any{"type": "string"}, "value": map[string]any{},
		"all": group, "any": group, "not": child,
	}, "description": "A predicate or all/any/not group. The parser enforces the published depth and clause bounds."}
}

// Subtitle languages come from current file evidence and the viewer's own or
// shared subtitle revisions. Private uploads never become another viewer's
// filter values, and obsolete revisions never claim a playable language.
func (c *browseCompiler) subtitleLanguagePredicate(field browseField, node BrowseNode) (string, error) {
	if node.Operator == "contains-all" {
		values, _ := node.Value.([]any)
		parts := []string{}
		for _, value := range values {
			clause, err := c.subtitleLanguagePredicate(field, BrowseNode{Field: field.ID, Operator: "equals", Value: value})
			if err != nil {
				return "", err
			}
			parts = append(parts, clause)
		}
		return "(" + strings.Join(parts, " AND ") + ")", nil
	}
	neg := node.Operator == "not-equals" || node.Operator == "not-in" || node.Operator == "is-missing"
	if neg {
		node.Operator = map[string]string{"not-equals": "equals", "not-in": "in", "is-missing": "is-present"}[node.Operator]
	}
	sources := []browseField{
		{ID: "subtitle-attribute", Type: valueSet, Join: attributeJoin("subtitleLanguage"), Value: "ca.source_value"},
		{ID: "subtitle-file", Type: valueSet, Join: `JOIN catalog_asset_links link ON link.entity_id=bei.item_id JOIN catalog_assets asset ON asset.id=link.asset_id JOIN asset_subtitle_facts facts ON facts.asset_id=asset.token AND facts.size=asset.size AND facts.modified_ns=asset.modified_ns AND facts.status='known' JOIN asset_subtitles track ON track.asset_id=asset.token`, Value: "track.language"},
		{ID: "subtitle-resource", Type: valueSet, Profile: 1, Join: `JOIN subtitle_resources resource ON resource.item_id=bei.item_id AND resource.deleted=0 AND (resource.scope='shared' OR resource.owner=?) JOIN subtitle_revisions revision ON revision.resource_id=resource.id AND revision.revision=resource.current_revision JOIN catalog_assets asset ON asset.token=resource.source_id AND asset.size=revision.source_size AND asset.modified_ns=revision.source_modified_ns`, Value: "revision.language"},
	}
	parts := []string{}
	for _, source := range sources {
		clause, err := c.exists(source, func() (string, error) { return c.comparison(source, source.Value, node.Operator, node.Value) })
		if err != nil {
			return "", err
		}
		parts = append(parts, clause)
	}
	clause := "(" + strings.Join(parts, " OR ") + ")"
	if neg {
		clause = "NOT " + clause
	}
	return clause, nil
}

// Container favourites and ratings belong to the title itself. Member state
// remains useful for aggregate views, but cannot replace that direct intent.
func (c *browseCompiler) personalTitlePredicate(field browseField, node BrowseNode) (string, error) {
	negative := false
	if field.Type == valueBoolean {
		value, _ := node.Value.(bool)
		negative = (node.Operator == "equals") != value
		node.Operator = "equals"
		node.Value = true
	} else if node.Operator == "not-equals" || node.Operator == "not-in" || node.Operator == "is-missing" {
		negative = true
		node.Operator = map[string]string{"not-equals": "equals", "not-in": "in", "is-missing": "is-present"}[node.Operator]
	}
	c.args = append(c.args, c.profile)
	own, err := c.comparison(field, field.Value, node.Operator, node.Value)
	if err != nil {
		return "", err
	}
	own = `EXISTS(SELECT 1 FROM personal_items pi WHERE pi.profile_id=? AND pi.item_id=e.entity_id AND ` + own + `)`
	memberField := field
	memberField.ID = "personal-member"
	member, err := c.itemPredicate(memberField, node)
	if err != nil {
		return "", err
	}
	clause := "(" + own + " OR " + member + ")"
	if negative {
		clause = "NOT " + clause
	}
	return clause, nil
}
