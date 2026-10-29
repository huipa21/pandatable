package formula

import (
	"strings"
	"testing"
)

func TestParseBindInferAndCompileFormula(t *testing.T) {
	tests := []struct {
		expression string
		fields     map[string]Type
		want       Type
		sqlParts   []string
	}{
		{
			expression: `{Price} * {Qty}`,
			fields:     map[string]Type{"price": Number, "qty": Number},
			want:       Number,
			sqlParts:   []string{"*"},
		},
		{
			expression: `IF({Status}="Done","✓","")`,
			fields:     map[string]Type{"status": Text},
			want:       Text,
			sqlParts:   []string{"CASE WHEN", "=", "✓"},
		},
		{
			expression: `CONCAT({First}," ",{Last})`,
			fields:     map[string]Type{"first": Text, "last": Text},
			want:       Text,
			sqlParts:   []string{"||", "::text"},
		},
		{
			expression: `DATETIME_DIFF(TODAY(),{Due},"days")`,
			fields:     map[string]Type{"due": Date},
			want:       Number,
			sqlParts:   []string{"make_interval(days", "Asia/Shanghai"},
		},
	}
	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			node, err := Parse(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			if err := BindFields(node, func(name string) (string, Type, error) {
				id := strings.ToLower(name)
				return id, test.fields[id], nil
			}); err != nil {
				t.Fatal(err)
			}
			got, err := Infer(node, func(id string) (Type, error) { return test.fields[id], nil })
			if err != nil || got != test.want {
				t.Fatalf("Infer() = %s, %v; want %s", got, err, test.want)
			}
			compiled, err := Compile(node, func(id string, _ *Compiler) (SQLValue, error) {
				return SQLValue{SQL: `source."` + id + `"`, Type: test.fields[id]}, nil
			}, "Asia/Shanghai")
			if err != nil {
				t.Fatal(err)
			}
			if compiled.Type != test.want {
				t.Fatalf("compiled = %+v", compiled)
			}
			for _, part := range test.sqlParts {
				if !strings.Contains(compiled.SQL, part) &&
					!containsString(compiled.Args, part) {
					t.Errorf("compiled SQL/args %q / %v do not contain %q", compiled.SQL, compiled.Args, part)
				}
			}
		})
	}
}

func TestParseAndTypeErrorsHavePositions(t *testing.T) {
	for _, test := range []struct {
		expression string
		resolve    TypeResolver
	}{
		{`1 +`, func(string) (Type, error) { return Null, nil }},
		{`{Missing} + 1`, func(string) (Type, error) { return Null, nil }},
		{`"text" * 2`, func(string) (Type, error) { return Null, nil }},
		{`UNKNOWN(1)`, func(string) (Type, error) { return Null, nil }},
	} {
		node, err := Parse(test.expression)
		if err == nil {
			_, err = Infer(node, test.resolve)
		}
		formulaError, ok := err.(*Error)
		if !ok || formulaError.Position < 1 {
			t.Errorf("%q error = %#v; expected positioned formula error", test.expression, err)
		}
	}
}

func TestCompilerBindsValuesAndMarksMaskedDependencies(t *testing.T) {
	node, err := Parse(`{Secret} + 3`)
	if err != nil {
		t.Fatal(err)
	}
	if err := BindFields(node, func(string) (string, Type, error) {
		return "secret-id", Number, nil
	}); err != nil {
		t.Fatal(err)
	}
	compiled, err := Compile(node, func(_ string, compiler *Compiler) (SQLValue, error) {
		return SQLValue{SQL: `source."secret"`, Type: Number, Masked: true}, nil
	}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.Masked || len(compiled.Args) != 1 || compiled.Args[0] != "3" {
		t.Fatalf("compiled = %+v", compiled)
	}
	if strings.Contains(compiled.SQL, "3") {
		t.Fatalf("literal was inlined: %s", compiled.SQL)
	}
}

func containsString(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
