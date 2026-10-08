package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

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
	profile string
	args    []any
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
	if field.Entity != "" {
		return c.entityPredicate(field, node)
	}
	return c.itemPredicate(field, node)
}

func collated(field browseField, expression string) string {
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

// comparison renders one operator against a value expression and binds its args.
func (c *browseCompiler) comparison(field browseField, expression, operator string, value any) (string, error) {
	target := collated(field, expression)
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
	return c.comparison(field, field.Entity, node.Operator, node.Value)
}

// exists wraps a member-item condition. Profile placeholders inside the join are
// bound before the condition's own arguments so ordinal positions line up.
func (c *browseCompiler) exists(field browseField, condition func() (string, error)) (string, error) {
	prefix := "EXISTS(SELECT 1 FROM catalog_browse_memberships bei " + field.Join + " WHERE bei.entity_id=e.entity_id AND "
	for i := 0; i < field.Profile; i++ {
		c.args = append(c.args, c.profile)
	}
	body, err := condition()
	if err != nil {
		return "", err
	}
	return prefix + body + ")", nil
}

func (c *browseCompiler) itemPredicate(field browseField, node BrowseNode) (string, error) {
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
