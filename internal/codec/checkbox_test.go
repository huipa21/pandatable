package codec

import (
	"encoding/json"
	"testing"

	"pandatable/internal/apperr"
)

func TestCheckboxOptions(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"icon":"check"}`,
		`{"icon":"square_check"}`,
		`{"icon":"circle_check"}`,
		`{"icon":"star"}`,
		`{"icon":"heart"}`,
		`{"icon":"thumbs_up"}`,
		`{"icon":"flag"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			field := mustBind(t, Checkbox, json.RawMessage(raw))
			if !sameJSON(t, field.Options(), json.RawMessage(raw)) {
				t.Fatalf("options %s, want %s", field.Options(), raw)
			}
			value, err := field.Decode(json.RawMessage(`true`))
			if err != nil || value != true {
				t.Fatalf("decode = %v, %v", value, err)
			}
		})
	}

	for _, test := range []struct {
		raw    string
		option string
	}{
		{`{"icon":"rocket"}`, "icon"},
		{`{"icon":null}`, "icon"},
		{`{"icon":"star","extra":true}`, "extra"},
		{`{"icon":"star","icon":"heart"}`, "icon"},
	} {
		t.Run(test.raw, func(t *testing.T) {
			_, err := Bind(Checkbox, json.RawMessage(test.raw))
			apiErr, ok := APIError(err).(*apperr.Error)
			if !ok || apiErr.Status != 400 || apiErr.Code != "ERR_VALIDATION" ||
				apiErr.Details["reason"] != ReasonInvalidOptions || apiErr.Details["option"] != test.option {
				t.Fatalf("error = %#v", APIError(err))
			}
		})
	}
}
