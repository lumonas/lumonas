package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lumonas/lumonas/internal/monitoring"
)

const scheduleSchema = `
CREATE TABLE IF NOT EXISTS job_schedules (
  id TEXT PRIMARY KEY,
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);`

func (s *Store) ensureScheduleSchema() error {
	_, err := s.db.Exec(scheduleSchema)
	return err
}

// JobSchedules returns every persisted schedule, seeding the defaults on first read.
func (s *Store) JobSchedules(now time.Time) ([]monitoring.Schedule, error) {
	if err := s.ensureScheduleSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT config_json FROM job_schedules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]monitoring.Schedule, 0)
	for rows.Next() {
		var encoded string
		var schedule monitoring.Schedule
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &schedule); err != nil {
			return nil, err
		}
		result = append(result, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		result = monitoring.DefaultSchedules()
		for _, schedule := range result {
			if err := s.SaveJobSchedule(schedule); err != nil {
				return nil, err
			}
		}
	}
	for index := range result {
		result[index].Humanize()
		result[index].Next = result[index].DescribeNext(now)
	}
	return result, nil
}

func (s *Store) JobSchedule(id string, now time.Time) (monitoring.Schedule, error) {
	schedules, err := s.JobSchedules(now)
	if err != nil {
		return monitoring.Schedule{}, err
	}
	for _, schedule := range schedules {
		if schedule.ID == id {
			return schedule, nil
		}
	}
	return monitoring.Schedule{}, sql.ErrNoRows
}

func (s *Store) SaveJobSchedule(schedule monitoring.Schedule) error {
	if err := schedule.Validate(); err != nil {
		return err
	}
	if err := s.ensureScheduleSchema(); err != nil {
		return err
	}
	schedule.Humanize()
	schedule.Next = ""
	encoded, err := json.Marshal(schedule)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO job_schedules(id,config_json,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`, schedule.ID, string(encoded), time.Now().UTC().Format(timeFormat))
	return err
}

func (s *Store) DeleteJobSchedule(id string) error {
	if err := s.ensureScheduleSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM job_schedules WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
