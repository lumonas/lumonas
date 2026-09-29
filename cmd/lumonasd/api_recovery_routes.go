package main

import (
	"net/http"
)

func (s *apiServer) apiRecoveryRoutes(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	switch {
	case r.Method == http.MethodGet && endpoint == "/recovery/status":
		s.recoveryStatus(w)
	case r.Method == http.MethodGet && endpoint == "/recovery/drills":
		s.restoreDrills(w, r)
	case r.Method == http.MethodPost && endpoint == "/recovery/drills/run":
		s.runRestoreDrillNow(w, r)
	case r.Method == http.MethodGet && endpoint == "/recovery/drills/schedule":
		s.restoreDrillSchedule(w, r)
	case r.Method == http.MethodGet && endpoint == "/recovery/workload-objectives":
		s.workloadRecoveryObjectives(w, r)
	case r.Method == http.MethodPut && endpoint == "/recovery/workload-objectives":
		s.updateWorkloadRecoveryObjective(w, r)
	case r.Method == http.MethodPatch && endpoint == "/recovery/drills/schedule":
		s.updateRestoreDrillSchedule(w, r)
	case r.Method == http.MethodPost && endpoint == "/recovery/key":
		s.createRecoveryKey(w, r)
	case r.Method == http.MethodGet && endpoint == "/recovery/plan":
		s.recoveryPlan(w)
	case r.Method == http.MethodGet && endpoint == "/recovery/download":
		s.downloadRecoveryBundle(w, r)
	case r.Method == http.MethodPost && endpoint == "/recovery/restore/stage":
		s.recoveryStage(w, r)
	case r.Method == http.MethodPost && endpoint == "/recovery/restore/stage-appdata":
		s.recoveryStageAppdata(w, r)
	case r.Method == http.MethodPost && endpoint == "/recovery/export":
		actor, ok := s.identityActor(w, r, true)
		if !ok {
			return true
		}
		s.recoveryExport(w, r.Context(), actor)
	default:
		return false
	}
	return true
}
