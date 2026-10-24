package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestDetectEngine(t *testing.T) {
	for _, test := range []struct {
		name, override, want string
		available            []string
		calls                []string
		wantErr, usage       bool
	}{
		{name: "prefer podman", available: []string{"podman", "docker"}, want: "podman", calls: []string{"podman"}},
		{name: "docker fallback", available: []string{"docker"}, want: "docker", calls: []string{"podman", "docker"}},
		{name: "override", override: "docker", available: []string{"podman", "docker"}, want: "docker", calls: []string{"docker"}},
		{name: "missing override", override: "docker", available: []string{"podman"}, calls: []string{"docker"}, wantErr: true},
		{name: "invalid override", override: "foo", wantErr: true, usage: true},
		{name: "none", calls: []string{"podman", "docker"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			got, err := detectEngine(test.override, func(name string) (string, error) {
				calls = append(calls, name)
				for _, available := range test.available {
					if name == available {
						return name, nil
					}
				}
				return "", errors.New("not found")
			})
			var usage usageError
			if (err != nil) != test.wantErr || got.name != test.want || errors.As(err, &usage) != test.usage || !reflect.DeepEqual(calls, test.calls) {
				t.Fatalf("got engine=%+v err=%v calls=%v", got, err, calls)
			}
		})
	}
}
