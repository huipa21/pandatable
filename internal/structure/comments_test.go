package structure

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestMentionNamesAndBodyBounds(t *testing.T) {
	names, err := mentionNames("Hi @Bob, @bob and @alice-smith; mail@example.test is not a mention")
	if err != nil || !reflect.DeepEqual(names, []string{"bob", "alice-smith"}) {
		t.Fatalf("mention names = %v, %v", names, err)
	}
	for _, body := range []string{"", " \n ", "bad\x00body", strings.Repeat("x", maxCommentBytes+1)} {
		if _, err := mentionNames(body); err == nil {
			t.Fatalf("accepted invalid comment %q", body)
		}
	}
	exactBytes := strings.Repeat("界", maxCommentBytes/3) + "x"
	if len(exactBytes) != maxCommentBytes {
		t.Fatal("invalid byte-boundary fixture")
	}
	if _, err := mentionNames(exactBytes); err != nil {
		t.Fatalf("rejected comment at the byte limit: %v", err)
	}
	if _, err := mentionNames(exactBytes + "x"); err == nil {
		t.Fatal("accepted comment exceeding the UTF-8 byte limit")
	}
	mentions := make([]string, maxMentions)
	for i := range mentions {
		mentions[i] = fmt.Sprintf("@user%02d", i)
	}
	if names, err := mentionNames(strings.Join(mentions, " ")); err != nil || len(names) != maxMentions {
		t.Fatalf("rejected %d mentions: %v", maxMentions, err)
	}
	if _, err := mentionNames(strings.Join(append(mentions, "@user50"), " ")); err == nil {
		t.Fatalf("accepted more than %d mentions", maxMentions)
	}
}
