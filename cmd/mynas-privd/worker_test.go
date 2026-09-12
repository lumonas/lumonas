package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBrokerRequiresConfirmationBeforeForwarding(t *testing.T) {
	result := executeBroker(request{Operation: "filesystem.format", PlanHash: "plan-1"})
	if result.OK || result.Error != "operation plan is not confirmed" {
		t.Fatalf("unexpected broker response: %#v", result)
	}
}

func TestWorkerRejectsOperationsOutsideItsCapabilityDomain(t *testing.T) {
	result := executeWorker(request{Operation: "power.action", PlanHash: "plan-1", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)}, "storage")
	if result.OK || result.Error != "operation is not allow-listed for this worker" {
		t.Fatalf("unexpected worker response: %#v", result)
	}
}

func TestBrokerForwardsConfirmedOperationToRootOnlyWorker(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "mw-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	t.Setenv("MYNAS_PRIVD_WORKER_DIR", directory)
	listener, err := net.Listen("unix", filepath.Join(directory, "power.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		var received request
		if err := json.NewDecoder(connection).Decode(&received); err != nil {
			done <- err
			return
		}
		if received.Operation != "power.action" {
			done <- &workerTestError{message: "unexpected operation"}
			return
		}
		done <- json.NewEncoder(connection).Encode(response{OK: true})
	}()
	result := executeBroker(request{Operation: "power.action", PlanHash: "plan-1", Confirmed: true, ExpiresAt: time.Now().UTC().Add(time.Minute)})
	if !result.OK {
		t.Fatalf("expected worker response, got %#v", result)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not receive forwarded request")
	}
}

type workerTestError struct{ message string }

func (e *workerTestError) Error() string { return e.message }
