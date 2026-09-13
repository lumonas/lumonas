package events

import "github.com/lumonas/lumonas/internal/model"

// Metadata contains the tracing fields that are promoted to the event
// envelope. Background publishers often put a Job in the payload, so the
// extraction intentionally understands that shape as well as the flat API
// shape.
type Metadata struct {
	CorrelationID string
	OperationID   string
	PlanHash      string
	Actor         string
	Generation    int64
}

func MetadataFromData(data map[string]any) Metadata {
	metadata := Metadata{
		CorrelationID: stringValue(data, "correlationId"),
		OperationID:   stringValue(data, "operationId"),
		PlanHash:      stringValue(data, "planHash"),
		Actor:         stringValue(data, "actor"),
		Generation:    int64Value(data, "generation"),
	}
	if job, ok := jobValue(data); ok {
		if metadata.CorrelationID == "" {
			metadata.CorrelationID = job.CorrelationID
		}
		if metadata.OperationID == "" {
			metadata.OperationID = job.OperationID
			if metadata.OperationID == "" {
				metadata.OperationID = job.ID
			}
		}
		if metadata.PlanHash == "" {
			metadata.PlanHash = job.PlanHash
		}
		if metadata.Generation == 0 {
			metadata.Generation = job.Generation
		}
		if metadata.Actor == "" {
			metadata.Actor = job.Actor
		}
	}
	if metadata.OperationID == "" {
		metadata.OperationID = stringValue(data, "jobId")
	}
	return metadata
}

func int64Value(data map[string]any, key string) int64 {
	if data == nil {
		return 0
	}
	switch value := data[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func stringValue(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func jobValue(data map[string]any) (model.Job, bool) {
	if data == nil {
		return model.Job{}, false
	}
	switch job := data["job"].(type) {
	case model.Job:
		return job, true
	case *model.Job:
		if job != nil {
			return *job, true
		}
	}
	return model.Job{}, false
}
