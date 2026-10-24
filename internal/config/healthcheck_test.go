package config

import (
	"errors"
	"flag"
	"testing"
)

func TestHealthcheckListen(t *testing.T) {
	for _, test := range []struct {
		name    string
		env     string
		args    []string
		want    string
		wantErr bool
	}{
		{"default", ":8080", nil, ":8080", false},
		{"environment", "127.0.0.1:9999", nil, "127.0.0.1:9999", false},
		{"flag overrides invalid env", "invalid", []string{"--listen", "[::]:8081"}, "[::]:8081", false},
		{"invalid", "invalid", nil, "", true},
		{"zero", ":0", nil, "", true},
		{"empty", "", nil, "", true},
		{"unknown", ":8080", []string{"--database-url", "secret"}, "", true},
		{"positional", ":8080", []string{"extra"}, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := healthcheckListen(test.args, func(name string) (string, bool) {
				return test.env, name == "PT_LISTEN"
			})
			if got != test.want || (err != nil) != test.wantErr {
				t.Fatalf("got %q, %v; want %q, error=%v", got, err, test.want, test.wantErr)
			}
		})
	}
	got, err := healthcheckListen(nil, func(string) (string, bool) { return "", false })
	if err != nil || got != ":8080" {
		t.Fatalf("default = %q, %v", got, err)
	}
	_, err = healthcheckListen([]string{"--help"}, func(string) (string, bool) { return "", false })
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help = %v", err)
	}
}
