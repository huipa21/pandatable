package structure

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"pandatable/internal/apperr"
	"pandatable/internal/formula"
)

func TestFormulaFunctionSQLExecutesOnPostgres(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		expression string
		want       string
	}{
		{`BLANK()`, `null`},
		{`IF(1=1,"yes","no")`, `"yes"`},
		{`AND(1=1,2=2)`, `true`},
		{`OR(1=2,2=2)`, `true`},
		{`NOT(1=2)`, `true`},
		{`SWITCH("x","x",1,0)`, `1`},
		{`ISBLANK(BLANK())`, `true`},
		{`COALESCE(BLANK(),"x")`, `"x"`},
		{`ROUND(2.35,1)`, `2.4`},
		{`ROUNDUP(-1.1)`, `-2`},
		{`ROUNDDOWN(-1.9)`, `-1`},
		{`ABS(-2)`, `2`},
		{`CEILING(-1.1)`, `-1`},
		{`FLOOR(-1.1)`, `-2`},
		{`MOD(5,2)`, `1`},
		{`POWER(2,3)`, `8`},
		{`SQRT(9)`, `3`},
		{`MIN(3,1,2)`, `1`},
		{`MAX(3,1,2)`, `3`},
		{`SUM(1,2,3)`, `6`},
		{`AVERAGE(1,2,3)`, `2`},
		{`INT(-1.9)`, `-1`},
		{`CONCAT("a","b")`, `"ab"`},
		{`LEN("😀")`, `1`},
		{`LEFT("abcd",2)`, `"ab"`},
		{`RIGHT("abcd",2)`, `"cd"`},
		{`MID("abcd",2,2)`, `"bc"`},
		{`UPPER("abc")`, `"ABC"`},
		{`LOWER("ABC")`, `"abc"`},
		{`TRIM("  a  ")`, `"a"`},
		{`SUBSTITUTE("banana","a","X")`, `"bXnXnX"`},
		{`SUBSTITUTE("banana","a","X",2)`, `"banXna"`},
		{`FIND("na","banana")`, `3`},
		{`REPLACE("banana",2,3,"X")`, `"bXna"`},
		{`T(123)`, `""`},
		{`VALUE("12.5")`, `12.5`},
		{`VALUE("not a number")`, `null`},
		{`REGEX_MATCH("abc","^a")`, `true`},
		{`NOW()`, ``},
		{`TODAY()`, ``},
		{`YEAR(DATE(2024,2,29))`, `2024`},
		{`MONTH(DATE(2024,2,29))`, `2`},
		{`DAY(DATE(2024,2,29))`, `29`},
		{`WEEKDAY(DATE(2024,2,29))`, `4`},
		{`HOUR(NOW())`, ``},
		{`MINUTE(NOW())`, ``},
		{`DATEADD(DATE(2024,2,28),1,"days")`, `"2024-02-29"`},
		{`DATETIME_DIFF(DATE(2024,3,1),DATE(2024,2,28),"days")`, `2`},
		{`DATETIME_FORMAT(DATE(2024,2,29),"YYYY-MM-DD")`, `"2024-02-29"`},
		{`DATE(2024,2,29)`, `"2024-02-29"`},
		{`"a" & "b"`, `"ab"`},
		{`2 + 3 * 4`, `14`},
		{`4 / 0`, `null`},
		{`5 % 2`, `1`},
		{`1 != 2`, `true`},
		{`1 < 2`, `true`},
		{`1 <= 1`, `true`},
		{`2 > 1`, `true`},
		{`2 >= 2`, `true`},
	}
	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			node, err := formula.Parse(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := formula.Infer(node, func(string) (formula.Type, error) {
				return formula.Null, errors.New("unexpected field reference")
			}); err != nil {
				t.Fatal(err)
			}
			compiled, err := formula.Compile(node, func(string, *formula.Compiler) (formula.SQLValue, error) {
				return formula.SQLValue{}, errors.New("unexpected field reference")
			}, "Asia/Shanghai")
			if err != nil {
				t.Fatal(err)
			}
			expression := compiled.SQL
			if compiled.Type == formula.Null {
				expression = "NULL::text"
			}
			var got []byte
			err = f.pool.QueryRow(f.ctx,
				"SELECT COALESCE(to_jsonb("+expression+"), 'null'::jsonb)",
				compiled.Args...).Scan(&got)
			if err != nil {
				t.Fatalf("execute compiled SQL %q: %v", compiled.SQL, err)
			}
			if test.want != "" && string(got) != test.want {
				var actual, expected any
				if err := json.Unmarshal(got, &actual); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(test.want), &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Errorf("value = %s; want %s", got, test.want)
				}
			}
		})
	}
}

func TestFormulaQueryValuesStableReferencesAndDependencyValidation(t *testing.T) {
	f := newFixture(t)
	owner := f.user("user", "active", "en")
	owner.Timezone = "Asia/Shanghai"
	workspace := f.workspace(owner)
	book := f.book(owner, workspace.ID)
	table := f.table(owner, book.ID)
	create := func(name, kind string) Field {
		t.Helper()
		result, err := f.service.CreateField(f.ctx, owner, table.Table.ID, FieldInput{Name: name, UIType: kind})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return *result.Field
	}
	price := create("Price", "decimal")
	qty := create("Qty", "integer")
	status := create("Status", "text")
	due := create("Due", "date")
	first := create("First", "text")
	last := create("Last", "text")
	newFormula := func(name, expression string) Field {
		t.Helper()
		options, _ := json.Marshal(map[string]string{"expr": expression})
		result, err := f.service.CreateField(f.ctx, owner, table.Table.ID, FieldInput{
			Name: name, UIType: "formula", Options: options,
		})
		if err != nil {
			t.Fatalf("create formula %s: %v", name, err)
		}
		return *result.Field
	}
	total := newFormula("Total", `{Price} * {Qty}`)
	completion := newFormula("Completion", `IF({Status}="Done","✓","")`)
	days := newFormula("Days", `DATETIME_DIFF(TODAY(), {Due}, "days")`)
	fullName := newFormula("Full name", `CONCAT({First}," ",{Last})`)
	dueDate := scanOne[string](f, `SELECT (now() AT TIME ZONE 'Asia/Shanghai')::date::text`)
	created, err := f.service.CreateRows(f.ctx, owner, table.Table.ID, []RowInsert{{
		Fields: map[string]json.RawMessage{
			price.ID: json.RawMessage(`12.50`), qty.ID: json.RawMessage(`4`),
			status.ID: json.RawMessage(`"Done"`), due.ID: json.RawMessage(`"` + dueDate + `"`),
			first.ID: json.RawMessage(`"Ada"`), last.ID: json.RawMessage(`"Lovelace"`),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var insertedTotal float64
	if err := json.Unmarshal(created[0].Fields[total.ID], &insertedTotal); err != nil || insertedTotal != 50 {
		t.Fatalf("formula value on inserted row = %v, %v", insertedTotal, err)
	}
	read := func() RowRecord {
		t.Helper()
		result, err := f.service.QueryRows(f.ctx, owner, table.Views[0].ID, RowQueryRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Rows) != 1 {
			t.Fatalf("rows = %d", len(result.Rows))
		}
		return result.Rows[0]
	}
	record := read()
	var gotTotal float64
	var gotStatus, gotName string
	var gotDays float64
	for _, check := range []struct {
		field string
		value any
	}{
		{total.ID, &gotTotal},
		{completion.ID, &gotStatus},
		{days.ID, &gotDays},
		{fullName.ID, &gotName},
	} {
		if err := json.Unmarshal(record.Fields[check.field], check.value); err != nil {
			t.Fatalf("decode %s: %v", check.field, err)
		}
	}
	if gotTotal != 50 || gotStatus != "✓" || gotDays != 0 || gotName != "Ada Lovelace" {
		t.Fatalf("formula values = %v, %q, %v, %q", gotTotal, gotStatus, gotDays, gotName)
	}

	renamed, err := f.service.UpdateField(f.ctx, owner, price.ID, FieldPatch{
		Version: price.Version, Name: stringPtr("Unit price"),
	})
	if err != nil {
		t.Fatal(err)
	}
	record = read()
	if err := json.Unmarshal(record.Fields[total.ID], &gotTotal); err != nil || gotTotal != 50 {
		t.Fatalf("formula after field rename = %v, %v", gotTotal, err)
	}
	checked, err := f.service.CheckFormula(f.ctx, owner, table.Table.ID, `{Unit price} * {Qty}`)
	if err != nil || checked.Type != "number" || checked.Position != 0 {
		t.Fatalf("formula check = %+v, %v", checked, err)
	}
	invalid, err := f.service.CheckFormula(f.ctx, owner, table.Table.ID, `{Unit price} +`)
	if err != nil || invalid.Position == 0 || invalid.Message == "" {
		t.Fatalf("invalid formula check = %+v, %v", invalid, err)
	}

	nested := newFormula("Nested", `{Total} + 1`)
	_, err = f.service.UpdateField(f.ctx, owner, nested.ID, FieldPatch{
		Version: nested.Version, Options: json.RawMessage(`{"expr":"{Nested} + 1"}`),
	})
	var apiError *apperr.Error
	if !errors.As(err, &apiError) || apiError.Code != "ERR_VALIDATION" {
		t.Fatalf("cyclic formula update error = %v", err)
	}
	_, err = f.service.DeleteField(f.ctx, owner, price.ID, renamed.Field.Version)
	if !errors.As(err, &apiError) || apiError.Code != "ERR_FIELD_IN_USE" {
		t.Fatalf("delete referenced field error = %v", err)
	}

	patched, err := f.service.PatchRow(f.ctx, owner, table.Table.ID, created[0].ID,
		map[string]json.RawMessage{status.ID: json.RawMessage(`"Open"`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(patched.Fields[completion.ID], &gotStatus); err != nil || gotStatus != "" {
		t.Fatalf("formula value on patched row = %q, %v", gotStatus, err)
	}
	record = read()
	if err := json.Unmarshal(record.Fields[completion.ID], &gotStatus); err != nil || gotStatus != "" {
		t.Fatalf("IF formula after related row update = %q, %v", gotStatus, err)
	}
	if renamed.Field == nil || renamed.Field.Name != "Unit price" {
		t.Fatalf("renamed field = %+v", renamed.Field)
	}
}
