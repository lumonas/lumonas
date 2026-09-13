package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lumonas/lumonas/internal/model"
)

// jobResourceKey is deliberately coarser than the job type. SnapRAID sync
// and scrub both mutate the same protection set, while SMART operations only
// need to exclude another operation against the same stable disk identity.
func jobResourceKey(jobType, resourceID string) string {
	switch {
	case strings.HasPrefix(jobType, "snapraid."):
		return "protection"
	case strings.HasPrefix(jobType, "smart."):
		return "disk:" + resourceID
	default:
		return jobType + ":" + resourceID
	}
}

type jobResourceBusyError struct {
	JobID string
}

func (e *jobResourceBusyError) Error() string {
	return fmt.Sprintf("job resource is busy (active job %s)", e.JobID)
}

func isJobResourceBusy(err error) bool {
	var busy *jobResourceBusyError
	return errors.As(err, &busy)
}

// admitJob performs the active-job check and insert under one process-wide
// mutex. SQLite serializes the write, but without this mutex two HTTP handlers
// could both observe an empty resource and enqueue conflicting work.
func (s *apiServer) admitJob(job model.Job) error {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()

	jobs, err := s.store.Jobs()
	if err != nil {
		return err
	}
	key := jobResourceKey(job.Type, job.ResourceID)
	for _, active := range jobs {
		if active.ID == job.ID || !jobIsActive(active.State) {
			continue
		}
		if jobResourceKey(active.Type, active.ResourceID) == key {
			return &jobResourceBusyError{JobID: active.ID}
		}
	}
	return s.store.SaveJob(job)
}

func jobIsActive(state string) bool {
	switch state {
	case "queued", "preparing", "running", "waiting-confirmation":
		return true
	default:
		return false
	}
}
