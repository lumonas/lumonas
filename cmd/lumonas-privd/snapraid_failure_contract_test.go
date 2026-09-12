package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSnapraidMutationsFailClosedWhenCommandFails(t *testing.T) {
	for _, operation := range []string{"snapraid.sync", "snapraid.scrub"} {
		t.Run(operation, func(t *testing.T) {
			var command string
			requested := map[string]any{"configPath": "/etc/lumonas/snapraid.conf"}
			if operation == "snapraid.scrub" {
				requested["scrubPercent"] = "10"
			}
			result := execute(request{
				Operation: operation, PlanHash: "failure-plan", RequestedState: requested,
				ExpiresAt: time.Now().UTC().Add(time.Minute), Confirmed: true,
			}, nil, func(name string, args ...string) ([]byte, error) {
				command = name + " " + strings.Join(args, " ")
				return []byte("configured disk is missing"), errors.New("exit status 1")
			})
			if result.OK || result.Error != "SnapRAID operation failed" {
				t.Fatalf("failed %s was not rejected: %#v", operation, result)
			}
			want := "snapraid -c /etc/lumonas/snapraid.conf " + strings.TrimPrefix(operation, "snapraid.")
			if operation == "snapraid.scrub" {
				want += " -p 10"
			}
			if command != want {
				t.Fatalf("unexpected SnapRAID command: got %q want %q", command, want)
			}
		})
	}
}
