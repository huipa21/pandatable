package codec

import (
	"encoding/json"
	"testing"
)

func TestAttachmentCodecOptionsAndValues(t *testing.T) {
	field, err := Bind(Attachment, json.RawMessage(`{"accept":" image/*, application/pdf ","maxFiles":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(field.Options()), `{"accept":"image/*, application/pdf","maxFiles":2}`; got != want {
		t.Fatalf("Options() = %s, want %s", got, want)
	}
	raw := json.RawMessage(`[ {"id":"123e4567-e89b-12d3-a456-426614174000","name":"photo.png","mime":"image/png","size":12,"thumb":"/api/v1/attachments/123e4567-e89b-12d3-a456-426614174000/thumbnail"} ]`)
	value, err := field.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := field.Encode(value)
	if err != nil || string(encoded) != `[{"id":"123e4567-e89b-12d3-a456-426614174000","name":"photo.png","mime":"image/png","size":12,"thumb":"/api/v1/attachments/123e4567-e89b-12d3-a456-426614174000/thumbnail"}]` {
		t.Fatalf("Encode() = %s, %v", encoded, err)
	}
}

func TestAttachmentCodecRejectsInvalidOptionsAndValues(t *testing.T) {
	for _, options := range []string{
		`{"maxFiles":0}`, `{"maxFiles":21}`, `{"accept":"*/*"}`,
		`{"accept":"image/*, image/*"}`, `{"extra":true}`,
	} {
		if _, err := Bind(Attachment, json.RawMessage(options)); err == nil {
			t.Errorf("Bind(%s) succeeded", options)
		}
	}
	field, err := Bind(Attachment, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`[{}]`,
		`[{"id":"123e4567-e89b-12d3-a456-426614174000","name":"x","mime":"image/svg+xml","size":0,"thumb":"/api/v1/attachments/123e4567-e89b-12d3-a456-426614174000/thumbnail"}]`,
		`[{"id":"123e4567-e89b-12d3-a456-426614174000","name":"x","mime":"image/png","size":0,"thumb":null},{"id":"123e4567-e89b-12d3-a456-426614174000","name":"y","mime":"image/png","size":0,"thumb":null}]`,
	} {
		if _, err := field.Decode(json.RawMessage(raw)); err == nil {
			t.Errorf("Decode(%s) succeeded", raw)
		}
	}
}

func TestAttachmentCodecPreservesValuesAfterOptionTightening(t *testing.T) {
	field, err := Bind(Attachment, json.RawMessage(`{"maxFiles":1}`))
	if err != nil {
		t.Fatal(err)
	}
	stored := json.RawMessage(`[{"id":"123e4567-e89b-12d3-a456-426614174000","name":"one.png","mime":"image/png","size":1,"thumb":null},{"id":"123e4567-e89b-12d3-a456-426614174001","name":"two.png","mime":"image/png","size":1,"thumb":null}]`)
	if _, err := field.Decode(stored); err == nil {
		t.Fatal("writing more than maxFiles succeeded")
	}
	if _, err := field.Encode(stored); err != nil {
		t.Fatalf("reading existing values after maxFiles was tightened: %v", err)
	}
	missingThumb := json.RawMessage(`[{"id":"123e4567-e89b-12d3-a456-426614174000","name":"one.png","mime":"image/png","size":1}]`)
	if _, err := field.Decode(missingThumb); err == nil {
		t.Fatal("missing thumb member was accepted")
	}
}
