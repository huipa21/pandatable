package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCheckFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"success", "failure", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var attempted []string
			var steps []checkStep
			failure := errors.New("lint failed")
			for _, name := range []string{"lint", "test", "build"} {
				steps = append(steps, checkStep{name, func(ctx context.Context) error {
					attempted = append(attempted, name)
					if name == "lint" {
						if mode == "failure" {
							return failure
						}
						if mode == "cancel" {
							cancel()
							return ctx.Err()
						}
					}
					return nil
				}})
			}
			err := runCheck(ctx, steps)
			want := []string{"lint", "test", "build"}
			if mode == "cancel" {
				want = want[:1]
			}
			if !reflect.DeepEqual(attempted, want) || (err == nil) != (mode == "success") {
				t.Fatalf("gate attempts=%v, error=%v", attempted, err)
			}
			if mode == "failure" && !errors.Is(err, failure) {
				t.Fatalf("gate lost lint failure: %v", err)
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("gate lost cancellation: %v", err)
			}
		})
	}
}
