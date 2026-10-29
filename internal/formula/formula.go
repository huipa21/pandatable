package formula

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Type string

const (
	Null     Type = "null"
	Number   Type = "number"
	Text     Type = "text"
	Boolean  Type = "bool"
	Date     Type = "date"
	DateTime Type = "datetime"
	Duration Type = "duration"
	Array    Type = "array"
)

type Node struct {
	Kind     string  `json:"kind"`
	Value    string  `json:"value,omitempty"`
	FieldID  string  `json:"fieldId,omitempty"`
	Position int     `json:"position"`
	Children []*Node `json:"children,omitempty"`
}

type Error struct {
	Position int
	Message  string
}

func (err *Error) Error() string { return err.Message }

type FieldResolver func(name string) (string, Type, error)

func BindFields(node *Node, resolve FieldResolver) error {
	if node.Kind == "field" {
		id, _, err := resolve(node.Value)
		if err != nil {
			return &Error{Position: node.Position, Message: err.Error()}
		}
		if id == "" {
			return &Error{Position: node.Position, Message: "Field reference is unavailable"}
		}
		node.FieldID = id
		return nil
	}
	for _, child := range node.Children {
		if err := BindFields(child, resolve); err != nil {
			return err
		}
	}
	return nil
}

func FieldIDs(node *Node) []string {
	seen := make(map[string]bool)
	var ids []string
	var visit func(*Node)
	visit = func(current *Node) {
		if current.Kind == "field" && current.FieldID != "" && !seen[current.FieldID] {
			seen[current.FieldID] = true
			ids = append(ids, current.FieldID)
		}
		for _, child := range current.Children {
			visit(child)
		}
	}
	visit(node)
	return ids
}

type token struct {
	kind string
	text string
	pos  int
}

func lex(source string) ([]token, error) {
	if !utf8.ValidString(source) {
		return nil, &Error{Position: 1, Message: "Formula must be valid UTF-8"}
	}
	tokens := make([]token, 0, len(source)/2)
	for index := 0; index < len(source); {
		r, size := utf8.DecodeRuneInString(source[index:])
		if unicode.IsSpace(r) {
			index += size
			continue
		}
		start := index
		switch {
		case r == '{':
			index += size
			nameStart := index
			for index < len(source) && source[index] != '}' {
				_, width := utf8.DecodeRuneInString(source[index:])
				index += width
			}
			if index == len(source) {
				return nil, &Error{Position: runePosition(source, start), Message: "Unclosed field reference"}
			}
			name := strings.TrimSpace(source[nameStart:index])
			index++
			if name == "" {
				return nil, &Error{Position: runePosition(source, start), Message: "Field reference cannot be empty"}
			}
			tokens = append(tokens, token{kind: "field", text: name, pos: runePosition(source, start)})
		case r == '"':
			index += size
			var value strings.Builder
			closed := false
			for index < len(source) {
				char, width := utf8.DecodeRuneInString(source[index:])
				index += width
				if char == '"' {
					if index < len(source) && source[index] == '"' {
						value.WriteByte('"')
						index++
						continue
					}
					closed = true
					break
				}
				if char == '\\' && index < len(source) {
					next, nextWidth := utf8.DecodeRuneInString(source[index:])
					index += nextWidth
					switch next {
					case 'n':
						value.WriteByte('\n')
					case 'r':
						value.WriteByte('\r')
					case 't':
						value.WriteByte('\t')
					default:
						value.WriteRune(next)
					}
					continue
				}
				value.WriteRune(char)
			}
			if !closed {
				return nil, &Error{Position: runePosition(source, start), Message: "Unclosed text literal"}
			}
			tokens = append(tokens, token{kind: "string", text: value.String(), pos: runePosition(source, start)})
		case unicode.IsDigit(r):
			index += size
			for index < len(source) {
				next, width := utf8.DecodeRuneInString(source[index:])
				if !unicode.IsDigit(next) && next != '.' {
					break
				}
				index += width
			}
			text := source[start:index]
			if _, err := strconv.ParseFloat(text, 64); err != nil {
				return nil, &Error{Position: runePosition(source, start), Message: "Invalid number"}
			}
			tokens = append(tokens, token{kind: "number", text: text, pos: runePosition(source, start)})
		case unicode.IsLetter(r) || r == '_':
			index += size
			for index < len(source) {
				next, width := utf8.DecodeRuneInString(source[index:])
				if !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_' {
					break
				}
				index += width
			}
			tokens = append(tokens, token{kind: "identifier", text: source[start:index], pos: runePosition(source, start)})
		default:
			operator := string(r)
			index += size
			if index < len(source) {
				pair := source[start : index+1]
				if pair == "!=" || pair == "<=" || pair == ">=" {
					operator = pair
					index++
				}
			}
			switch operator {
			case "+", "-", "*", "/", "%", "&", "=", "!=", "<", "<=", ">", ">=", "(", ")", ",":
				tokens = append(tokens, token{kind: operator, text: operator, pos: runePosition(source, start)})
			default:
				return nil, &Error{Position: runePosition(source, start), Message: "Unexpected character"}
			}
		}
	}
	tokens = append(tokens, token{kind: "eof", pos: runePosition(source, len(source))})
	return tokens, nil
}

func runePosition(value string, byteOffset int) int {
	return utf8.RuneCountInString(value[:byteOffset]) + 1
}

type parser struct {
	tokens []token
	index  int
}

func Parse(source string) (*Node, error) {
	if strings.TrimSpace(source) == "" {
		return nil, &Error{Position: 1, Message: "Formula cannot be empty"}
	}
	if len(source) > 16_384 {
		return nil, &Error{Position: 1, Message: "Formula is too long"}
	}
	tokens, err := lex(source)
	if err != nil {
		return nil, err
	}
	p := parser{tokens: tokens}
	root, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	if current := p.current(); current.kind != "eof" {
		return nil, &Error{Position: current.pos, Message: "Unexpected token"}
	}
	return root, nil
}

func (p *parser) current() token { return p.tokens[p.index] }

func (p *parser) take() token {
	current := p.current()
	if current.kind != "eof" {
		p.index++
	}
	return current
}

func precedence(kind string) int {
	switch kind {
	case "=", "!=", "<", "<=", ">", ">=":
		return 1
	case "&":
		return 2
	case "+", "-":
		return 3
	case "*", "/", "%":
		return 4
	default:
		return 0
	}
}

func (p *parser) expression(minimum int) (*Node, error) {
	current := p.take()
	var left *Node
	switch current.kind {
	case "number", "string":
		left = &Node{Kind: current.kind, Value: current.text, Position: current.pos}
	case "field":
		left = &Node{Kind: "field", Value: current.text, Position: current.pos}
	case "-":
		child, err := p.expression(5)
		if err != nil {
			return nil, err
		}
		left = &Node{Kind: "unary", Value: "-", Position: current.pos, Children: []*Node{child}}
	case "identifier":
		name := strings.ToUpper(current.text)
		if name == "NOT" {
			child, err := p.expression(5)
			if err != nil {
				return nil, err
			}
			left = &Node{Kind: "unary", Value: "NOT", Position: current.pos, Children: []*Node{child}}
			break
		}
		if p.current().kind != "(" {
			return nil, &Error{Position: current.pos, Message: "Function call expected"}
		}
		p.take()
		args := make([]*Node, 0, 3)
		if p.current().kind != ")" {
			for {
				arg, err := p.expression(0)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if p.current().kind != "," {
					break
				}
				p.take()
			}
		}
		if p.current().kind != ")" {
			return nil, &Error{Position: p.current().pos, Message: "Expected )"}
		}
		p.take()
		left = &Node{Kind: "call", Value: name, Position: current.pos, Children: args}
	case "(":
		var err error
		left, err = p.expression(0)
		if err != nil {
			return nil, err
		}
		if p.current().kind != ")" {
			return nil, &Error{Position: p.current().pos, Message: "Expected )"}
		}
		p.take()
	default:
		return nil, &Error{Position: current.pos, Message: "Expected a value"}
	}
	for precedence(p.current().kind) > minimum {
		op := p.take()
		right, err := p.expression(precedence(op.kind))
		if err != nil {
			return nil, err
		}
		left = &Node{Kind: "binary", Value: op.kind, Position: op.pos, Children: []*Node{left, right}}
	}
	return left, nil
}

type TypeResolver func(fieldID string) (Type, error)

func Infer(node *Node, resolve TypeResolver) (Type, error) {
	if node == nil {
		return Null, &Error{Position: 1, Message: "Formula is missing"}
	}
	switch node.Kind {
	case "number":
		return Number, nil
	case "string":
		return Text, nil
	case "field":
		if node.FieldID == "" {
			return Null, &Error{Position: node.Position, Message: "Field reference is unresolved"}
		}
		result, err := resolve(node.FieldID)
		if err != nil {
			return Null, at(node, err)
		}
		return result, nil
	case "unary":
		typeOf, err := inferChild(node.Children[0], resolve)
		if err != nil {
			return Null, err
		}
		if node.Value == "-" && typeOf != Number || node.Value == "NOT" && typeOf != Boolean {
			return Null, &Error{Position: node.Position, Message: "Operator does not accept this type"}
		}
		if node.Value == "-" {
			return Number, nil
		}
		return Boolean, nil
	case "binary":
		left, err := inferChild(node.Children[0], resolve)
		if err != nil {
			return Null, err
		}
		right, err := inferChild(node.Children[1], resolve)
		if err != nil {
			return Null, err
		}
		switch node.Value {
		case "+", "-", "*", "/", "%":
			if left != Number || right != Number {
				return Null, &Error{Position: node.Position, Message: "Arithmetic operators require numbers"}
			}
			return Number, nil
		case "&":
			if left != Text && left != Null || right != Text && right != Null {
				return Null, &Error{Position: node.Position, Message: "Text concatenation requires text"}
			}
			return Text, nil
		default:
			if !comparable(left, right) {
				return Null, &Error{Position: node.Position, Message: "Comparison requires compatible values"}
			}
			if strings.Contains("<>=", node.Value) || node.Value == "!=" {
				if left == Boolean || right == Boolean || left == Array || right == Array {
					if node.Value != "=" && node.Value != "!=" {
						return Null, &Error{Position: node.Position, Message: "Values do not support ordering"}
					}
				}
			}
			return Boolean, nil
		}
	case "call":
		args := make([]Type, len(node.Children))
		for index, child := range node.Children {
			value, err := inferChild(child, resolve)
			if err != nil {
				return Null, err
			}
			args[index] = value
		}
		return inferCall(node, args)
	default:
		return Null, &Error{Position: node.Position, Message: "Invalid formula expression"}
	}
}

func inferChild(node *Node, resolve TypeResolver) (Type, error) { return Infer(node, resolve) }

func at(node *Node, err error) error {
	var formulaError *Error
	if errors.As(err, &formulaError) {
		return err
	}
	return &Error{Position: node.Position, Message: err.Error()}
}

func comparable(left, right Type) bool {
	return left == right || left == Null || right == Null
}

func inferCall(node *Node, args []Type) (Type, error) {
	fail := func(message string) (Type, error) {
		return Null, &Error{Position: node.Position, Message: message}
	}
	all := func(expected Type) bool {
		for _, arg := range args {
			if arg != expected && arg != Null {
				return false
			}
		}
		return true
	}
	common := func(values []Type) Type {
		result := Null
		for _, arg := range values {
			if arg == Null {
				continue
			}
			if result != Null && result != arg {
				return Null
			}
			result = arg
		}
		return result
	}
	switch node.Value {
	case "BLANK":
		if len(args) != 0 {
			return fail("BLANK takes no arguments")
		}
		return Null, nil
	case "NOW":
		if len(args) != 0 {
			return fail("NOW takes no arguments")
		}
		return DateTime, nil
	case "TODAY":
		if len(args) != 0 {
			return fail("TODAY takes no arguments")
		}
		return Date, nil
	case "IF":
		if len(args) != 3 || args[0] != Boolean {
			return fail("IF requires a condition and two compatible values")
		}
		result := common(args[1:])
		if result == Null && args[1] != Null && args[2] != Null {
			return fail("IF result values must have the same type")
		}
		return result, nil
	case "AND", "OR":
		if len(args) < 2 || !all(Boolean) {
			return fail(node.Value + " requires at least two conditions")
		}
		return Boolean, nil
	case "NOT":
		if len(args) != 1 || !all(Boolean) {
			return fail("NOT requires one condition")
		}
		return Boolean, nil
	case "SWITCH":
		if len(args) < 4 || len(args)%2 != 0 {
			return fail("SWITCH requires a value, match/result pairs, and a default")
		}
		for index := 1; index < len(args)-1; index += 2 {
			if !comparable(args[0], args[index]) {
				return fail("SWITCH match values must match the input type")
			}
		}
		values := make([]Type, 0, len(args)/2)
		for index := 2; index < len(args)-1; index += 2 {
			values = append(values, args[index])
		}
		values = append(values, args[len(args)-1])
		result := common(values)
		if result == Null && commonNonNull(values) {
			return fail("SWITCH result values must have the same type")
		}
		return result, nil
	case "ISBLANK":
		if len(args) != 1 {
			return fail("ISBLANK requires one value")
		}
		return Boolean, nil
	case "COALESCE":
		if len(args) < 2 {
			return fail("COALESCE requires at least two values")
		}
		result := common(args)
		if result == Null && commonNonNull(args) {
			return fail("COALESCE values must have the same type")
		}
		return result, nil
	case "ROUND", "ROUNDUP", "ROUNDDOWN":
		if !numericArgs(args, 1, 2) {
			return fail(node.Value + " requires one or two numbers")
		}
		return Number, nil
	case "ABS", "CEILING", "FLOOR", "SQRT", "INT":
		if !numericArgs(args, 1, 1) {
			return fail(node.Value + " requires one number")
		}
		return Number, nil
	case "MOD", "POWER":
		if !numericArgs(args, 2, 2) {
			return fail(node.Value + " requires two numbers")
		}
		return Number, nil
	case "MIN", "MAX", "SUM", "AVERAGE":
		if !numericArgs(args, 1, -1) {
			return fail(node.Value + " requires one or more numbers")
		}
		return Number, nil
	case "CONCAT":
		if len(args) == 0 {
			return fail("CONCAT requires at least one value")
		}
		return Text, nil
	case "LEN", "VALUE", "UPPER", "LOWER", "TRIM", "T":
		if len(args) != 1 {
			return fail(node.Value + " requires one value")
		}
		if node.Value == "VALUE" {
			if args[0] != Text && args[0] != Null {
				return fail("VALUE requires text")
			}
			return Number, nil
		}
		if node.Value == "T" {
			return Text, nil
		}
		if args[0] != Text && args[0] != Null {
			return fail(node.Value + " requires text")
		}
		if node.Value == "LEN" {
			return Number, nil
		}
		return Text, nil
	case "LEFT", "RIGHT":
		if len(args) != 2 || (args[0] != Text && args[0] != Null) || args[1] != Number {
			return fail(node.Value + " requires text and a number")
		}
		return Text, nil
	case "MID", "REPLACE":
		if len(args) != 3 && !(node.Value == "REPLACE" && len(args) == 4) {
			return fail(node.Value + " has the wrong number of arguments")
		}
		if (args[0] != Text && args[0] != Null) || args[1] != Number || args[2] != Number {
			return fail(node.Value + " requires text and numeric positions")
		}
		if node.Value == "REPLACE" {
			if args[3] != Text && args[3] != Null {
				return fail("REPLACE requires replacement text")
			}
			return Text, nil
		}
		return Text, nil
	case "SUBSTITUTE":
		if len(args) != 3 && len(args) != 4 {
			return fail("SUBSTITUTE requires three or four text values")
		}
		for _, arg := range args {
			if arg != Text && arg != Null && arg != Number {
				return fail("SUBSTITUTE arguments must be text or numeric")
			}
		}
		return Text, nil
	case "FIND":
		if len(args) < 2 || len(args) > 3 || !all(Text) {
			return fail("FIND requires two or three text values")
		}
		return Number, nil
	case "REGEX_MATCH":
		if len(args) != 2 || !all(Text) {
			return fail("REGEX_MATCH requires two text values")
		}
		return Boolean, nil
	case "YEAR", "MONTH", "DAY", "WEEKDAY", "HOUR", "MINUTE":
		if len(args) != 1 || (args[0] != Date && args[0] != DateTime && args[0] != Null) {
			return fail(node.Value + " requires a date or datetime")
		}
		return Number, nil
	case "DATEADD":
		if len(args) != 3 || (args[0] != Date && args[0] != DateTime) || args[1] != Number || args[2] != Text {
			return fail("DATEADD requires a date, number and unit")
		}
		return args[0], nil
	case "DATETIME_DIFF":
		if len(args) != 3 || !dateLike(args[0]) || !dateLike(args[1]) || args[2] != Text {
			return fail("DATETIME_DIFF requires two dates and a unit")
		}
		return Number, nil
	case "DATETIME_FORMAT":
		if len(args) != 2 || !dateLike(args[0]) || args[1] != Text {
			return fail("DATETIME_FORMAT requires a date and format")
		}
		return Text, nil
	case "DATE":
		if !numericArgs(args, 3, 3) {
			return fail("DATE requires year, month and day numbers")
		}
		return Date, nil
	default:
		return fail("Unknown function " + node.Value)
	}
}

func commonNonNull(types []Type) bool {
	expected := Null
	for _, current := range types {
		if current == Null {
			continue
		}
		if expected != Null && current != expected {
			return true
		}
		expected = current
	}
	return false
}

func dateLike(value Type) bool { return value == Date || value == DateTime || value == Null }

func numericArgs(types []Type, min, max int) bool {
	if len(types) < min || max >= 0 && len(types) > max {
		return false
	}
	for _, value := range types {
		if value != Number && value != Null {
			return false
		}
	}
	return true
}

type SQLValue struct {
	SQL    string
	Type   Type
	Masked bool
}

type Resolver func(fieldID string, compiler *Compiler) (SQLValue, error)

type Compiler struct {
	args     []any
	resolver Resolver
	timezone string
}

type Compiled struct {
	SQL    string
	Args   []any
	Type   Type
	Masked bool
}

func Compile(node *Node, resolver Resolver, timezone string) (Compiled, error) {
	if resolver == nil {
		return Compiled{}, errors.New("formula compiler requires a field resolver")
	}
	compiler := &Compiler{resolver: resolver, timezone: timezone}
	value, err := compiler.CompileChild(node)
	if err != nil {
		return Compiled{}, err
	}
	return Compiled{SQL: value.SQL, Args: compiler.args, Type: value.Type, Masked: value.Masked}, nil
}

func (compiler *Compiler) Bind(value any) string {
	compiler.args = append(compiler.args, value)
	return fmt.Sprintf("$%d", len(compiler.args))
}

func (compiler *Compiler) CompileChild(node *Node) (SQLValue, error) {
	switch node.Kind {
	case "number":
		return SQLValue{SQL: compiler.Bind(node.Value) + "::numeric", Type: Number}, nil
	case "string":
		return SQLValue{SQL: compiler.Bind(node.Value) + "::text", Type: Text}, nil
	case "field":
		value, err := compiler.resolver(node.FieldID, compiler)
		if err != nil {
			return SQLValue{}, at(node, err)
		}
		if value.SQL == "" {
			return SQLValue{}, &Error{Position: node.Position, Message: "Field reference is unavailable"}
		}
		return value, nil
	case "unary":
		value, err := compiler.CompileChild(node.Children[0])
		if err != nil {
			return SQLValue{}, err
		}
		if node.Value == "NOT" {
			value.SQL = "(NOT (" + value.SQL + "))"
		} else {
			value.SQL = "(-(" + value.SQL + "))"
		}
		return value, nil
	case "binary":
		left, err := compiler.CompileChild(node.Children[0])
		if err != nil {
			return SQLValue{}, err
		}
		right, err := compiler.CompileChild(node.Children[1])
		if err != nil {
			return SQLValue{}, err
		}
		op := node.Value
		if op == "&" {
			op = "||"
		}
		if op == "/" {
			right.SQL = "NULLIF((" + right.SQL + "), 0)"
		}
		typeOf := Boolean
		if strings.Contains("+-*/%", node.Value) {
			typeOf = Number
		} else if node.Value == "&" {
			typeOf = Text
		}
		return SQLValue{SQL: "(" + left.SQL + " " + op + " " + right.SQL + ")", Type: typeOf,
			Masked: left.Masked || right.Masked}, nil
	case "call":
		args := make([]SQLValue, len(node.Children))
		types := make([]Type, len(node.Children))
		for index, child := range node.Children {
			value, err := compiler.CompileChild(child)
			if err != nil {
				return SQLValue{}, err
			}
			args[index], types[index] = value, value.Type
		}
		resultType, err := inferCall(node, types)
		if err != nil {
			return SQLValue{}, err
		}
		sql, err := compiler.compileCall(node.Value, args)
		if err != nil {
			return SQLValue{}, err
		}
		masked := false
		for _, value := range args {
			masked = masked || value.Masked
		}
		return SQLValue{SQL: sql, Type: resultType, Masked: masked}, nil
	default:
		return SQLValue{}, &Error{Position: node.Position, Message: "Invalid formula expression"}
	}
}

func (compiler *Compiler) compileCall(name string, args []SQLValue) (string, error) {
	sqlArgs := make([]string, len(args))
	for index, arg := range args {
		sqlArgs[index] = "(" + arg.SQL + ")"
	}
	switch name {
	case "BLANK":
		return "NULL", nil
	case "NOW":
		return "now()", nil
	case "TODAY":
		return "(now() AT TIME ZONE " + compiler.Bind(compiler.timezone) + ")::date", nil
	case "IF":
		return "(CASE WHEN " + sqlArgs[0] + " THEN " + sqlArgs[1] + " ELSE " + sqlArgs[2] + " END)", nil
	case "AND", "OR":
		return "(" + strings.Join(sqlArgs, " "+strings.ToLower(name)+" ") + ")", nil
	case "NOT":
		return "(NOT " + sqlArgs[0] + ")", nil
	case "SWITCH":
		var result strings.Builder
		result.WriteString("(CASE ")
		result.WriteString(sqlArgs[0])
		for index := 1; index < len(sqlArgs)-1; index += 2 {
			result.WriteString(" WHEN ")
			result.WriteString(sqlArgs[index])
			result.WriteString(" THEN ")
			result.WriteString(sqlArgs[index+1])
		}
		result.WriteString(" ELSE ")
		result.WriteString(sqlArgs[len(sqlArgs)-1])
		result.WriteString(" END)")
		return result.String(), nil
	case "ISBLANK":
		return "(" + sqlArgs[0] + " IS NULL OR " + sqlArgs[0] + "::text = '')", nil
	case "COALESCE":
		return "COALESCE(" + strings.Join(sqlArgs, ", ") + ")", nil
	case "ROUND":
		if len(args) == 1 {
			return "round(" + sqlArgs[0] + ")", nil
		}
		return "round(" + sqlArgs[0] + ", " + sqlArgs[1] + "::int)", nil
	case "ROUNDUP":
		scale := "0"
		if len(args) == 2 {
			scale = sqlArgs[1] + "::int"
		}
		return "CASE WHEN " + sqlArgs[0] + " >= 0 THEN ceil(" + sqlArgs[0] + " * power(10, " + scale + ")) / power(10, " + scale +
			") ELSE floor(" + sqlArgs[0] + " * power(10, " + scale + ")) / power(10, " + scale + ") END", nil
	case "ROUNDDOWN":
		scale := "0"
		if len(args) == 2 {
			scale = sqlArgs[1] + "::int"
		}
		return "trunc(" + sqlArgs[0] + ", " + scale + ")", nil
	case "ABS", "CEILING", "FLOOR", "SQRT", "INT":
		function := map[string]string{"CEILING": "ceil", "INT": "trunc"}[name]
		if function == "" {
			function = strings.ToLower(name)
		}
		return function + "(" + sqlArgs[0] + ")", nil
	case "MOD":
		return "mod(" + strings.Join(sqlArgs, ", ") + ")", nil
	case "POWER":
		return "power(" + strings.Join(sqlArgs, ", ") + ")", nil
	case "MIN", "MAX":
		function := "least"
		if name == "MAX" {
			function = "greatest"
		}
		return function + "(" + strings.Join(sqlArgs, ", ") + ")", nil
	case "SUM":
		return "(" + strings.Join(sqlArgs, " + ") + ")", nil
	case "AVERAGE":
		return "((" + strings.Join(sqlArgs, " + ") + ") / " + strconv.Itoa(len(args)) + ")", nil
	case "CONCAT":
		for index := range sqlArgs {
			sqlArgs[index] += "::text"
		}
		return "(" + strings.Join(sqlArgs, " || ") + ")", nil
	case "LEN":
		return "char_length(" + sqlArgs[0] + ")", nil
	case "LEFT", "RIGHT":
		return strings.ToLower(name) + "(" + sqlArgs[0] + ", greatest(0, " + sqlArgs[1] + "::int))", nil
	case "MID":
		return "substring(" + sqlArgs[0] + " from greatest(1, " + sqlArgs[1] + "::int) for greatest(0, " + sqlArgs[2] + "::int))", nil
	case "UPPER", "LOWER", "TRIM":
		return strings.ToLower(name) + "(" + sqlArgs[0] + ")", nil
	case "SUBSTITUTE":
		if len(args) == 3 {
			return "replace(" + sqlArgs[0] + ", " + sqlArgs[1] + "::text, " + sqlArgs[2] + "::text)", nil
		}
		return "(CASE WHEN " + sqlArgs[1] + "::text = '' THEN " + sqlArgs[0] +
			" WHEN " + sqlArgs[3] + "::int <= 0 THEN replace(" + sqlArgs[0] + "::text, " + sqlArgs[1] +
			"::text, " + sqlArgs[2] + "::text) ELSE (SELECT string_agg(piece, CASE WHEN ordinal = " +
			sqlArgs[3] + "::int + 1 THEN " + sqlArgs[2] + "::text ELSE " + sqlArgs[1] +
			"::text END ORDER BY ordinal) FROM unnest(string_to_array(" + sqlArgs[0] + "::text, " +
			sqlArgs[1] + "::text)) WITH ORDINALITY AS parts(piece, ordinal)) END)", nil
	case "FIND":
		start := "1"
		if len(args) == 3 {
			start = "greatest(1, " + sqlArgs[2] + "::int)"
		}
		return "COALESCE(NULLIF(strpos(substring(" + sqlArgs[1] + " from " + start + "), " + sqlArgs[0] +
			"), 0) + " + start + " - 1, 0)", nil
	case "REPLACE":
		from := "greatest(1, " + sqlArgs[1] + "::int)"
		return "(substring(" + sqlArgs[0] + " from 1 for " + from + " - 1) || " + sqlArgs[3] +
			" || substring(" + sqlArgs[0] + " from " + from + " + greatest(0, " + sqlArgs[2] + "::int)))", nil
	case "T":
		if args[0].Type == Text || args[0].Type == Null {
			return "COALESCE(" + sqlArgs[0] + "::text, '')", nil
		}
		return "(CASE WHEN " + sqlArgs[0] + " IS NULL THEN ''::text ELSE ''::text END)", nil
	case "VALUE":
		text := "(" + sqlArgs[0] + "::text)"
		// Volatile identity prevents custom plans from folding a bad cast before CASE.
		guarded := "(" + text + " || substring(clock_timestamp()::text FROM 1 FOR 0))"
		return "CASE WHEN pg_input_is_valid(" + text + ", 'numeric') THEN " + guarded + "::numeric END", nil
	case "REGEX_MATCH":
		return "(" + sqlArgs[0] + "::text ~ " + sqlArgs[1] + "::text)", nil
	case "YEAR", "MONTH", "DAY", "WEEKDAY", "HOUR", "MINUTE":
		part := map[string]string{"YEAR": "year", "MONTH": "month", "DAY": "day", "WEEKDAY": "dow",
			"HOUR": "hour", "MINUTE": "minute"}[name]
		dateSQL := sqlArgs[0]
		if args[0].Type == DateTime {
			dateSQL = "(" + dateSQL + " AT TIME ZONE " + compiler.Bind(compiler.timezone) + ")"
		}
		return "extract(" + part + " from " + dateSQL + ")", nil
	case "DATEADD":
		timezone := ""
		if args[0].Type == DateTime {
			timezone = " AT TIME ZONE " + compiler.Bind(compiler.timezone)
			return "((" + dateAddInterval(sqlArgs[2], sqlArgs[1]) + " + (" + sqlArgs[0] + timezone + ")) AT TIME ZONE " +
				compiler.Bind(compiler.timezone) + ")", nil
		}
		return "(" + sqlArgs[0] + " + " + dateAddInterval(sqlArgs[2], sqlArgs[1]) + ")::date", nil
	case "DATETIME_DIFF":
		var diff string
		if args[0].Type == Date && args[1].Type == Date {
			diff = "make_interval(days => ((" + sqlArgs[0] + ")::date - (" + sqlArgs[1] + ")::date))"
		} else {
			first, second := sqlArgs[0], sqlArgs[1]
			if args[0].Type == DateTime {
				first = "(" + first + " AT TIME ZONE " + compiler.Bind(compiler.timezone) + ")"
			} else {
				first = "(" + first + ")::timestamp"
			}
			if args[1].Type == DateTime {
				second = "(" + second + " AT TIME ZONE " + compiler.Bind(compiler.timezone) + ")"
			} else {
				second = "(" + second + ")::timestamp"
			}
			diff = "(" + first + " - " + second + ")"
		}
		return dateDiff(sqlArgs[2], diff), nil
	case "DATETIME_FORMAT":
		value := sqlArgs[0]
		if args[0].Type == DateTime {
			value = "(" + value + " AT TIME ZONE " + compiler.Bind(compiler.timezone) + ")"
		}
		return "to_char(" + value + ", " + sqlArgs[1] + "::text)", nil
	case "DATE":
		return "make_date(" + sqlArgs[0] + "::int, " + sqlArgs[1] + "::int, " + sqlArgs[2] + "::int)", nil
	default:
		return "", &Error{Message: "Unknown function " + name}
	}
}

func dateAddInterval(unit, amount string) string {
	return "(CASE lower(" + unit + "::text) " +
		"WHEN 'year' THEN make_interval(years => " + amount + "::int) " +
		"WHEN 'years' THEN make_interval(years => " + amount + "::int) " +
		"WHEN 'month' THEN make_interval(months => " + amount + "::int) " +
		"WHEN 'months' THEN make_interval(months => " + amount + "::int) " +
		"WHEN 'week' THEN make_interval(days => (" + amount + "::int * 7)) " +
		"WHEN 'weeks' THEN make_interval(days => (" + amount + "::int * 7)) " +
		"WHEN 'day' THEN make_interval(days => " + amount + "::int) " +
		"WHEN 'days' THEN make_interval(days => " + amount + "::int) " +
		"WHEN 'hour' THEN make_interval(hours => " + amount + "::int) " +
		"WHEN 'hours' THEN make_interval(hours => " + amount + "::int) " +
		"WHEN 'minute' THEN make_interval(mins => " + amount + "::int) " +
		"WHEN 'minutes' THEN make_interval(mins => " + amount + "::int) " +
		"WHEN 'second' THEN make_interval(secs => " + amount + ") " +
		"WHEN 'seconds' THEN make_interval(secs => " + amount + ") ELSE NULL END)"
}

func dateDiff(unit, difference string) string {
	seconds := "extract(epoch from " + difference + ")"
	return "(CASE lower(" + unit + "::text) " +
		"WHEN 'year' THEN " + seconds + " / 31557600 WHEN 'years' THEN " + seconds + " / 31557600 " +
		"WHEN 'month' THEN " + seconds + " / 2629800 WHEN 'months' THEN " + seconds + " / 2629800 " +
		"WHEN 'week' THEN " + seconds + " / 604800 WHEN 'weeks' THEN " + seconds + " / 604800 " +
		"WHEN 'day' THEN " + seconds + " / 86400 WHEN 'days' THEN " + seconds + " / 86400 " +
		"WHEN 'hour' THEN " + seconds + " / 3600 WHEN 'hours' THEN " + seconds + " / 3600 " +
		"WHEN 'minute' THEN " + seconds + " / 60 WHEN 'minutes' THEN " + seconds + " / 60 " +
		"WHEN 'second' THEN " + seconds + " WHEN 'seconds' THEN " + seconds + " ELSE NULL END)"
}
