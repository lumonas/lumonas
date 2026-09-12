package store

import (
	"database/sql"
	"time"

	"github.com/lumonas/lumonas/internal/notify"
)

const notificationSchema = `
CREATE TABLE IF NOT EXISTS notification_channels (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  label TEXT NOT NULL,
  target TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  credentials_ciphertext BLOB,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS notification_deliveries (
  id TEXT PRIMARY KEY, channel_id TEXT NOT NULL, event_type TEXT NOT NULL,
  state TEXT NOT NULL, attempted_at TEXT NOT NULL, error TEXT
);
CREATE INDEX IF NOT EXISTS notification_deliveries_attempted_idx ON notification_deliveries(attempted_at);`

func (s *Store) ensureNotificationSchema() error {
	_, err := s.db.Exec(notificationSchema)
	return err
}

func (s *Store) ListNotificationChannels() ([]notify.Channel, error) {
	if err := s.ensureNotificationSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,type,label,target,enabled,credentials_ciphertext,created_at,updated_at FROM notification_channels ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]notify.Channel, 0)
	for rows.Next() {
		value, _, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) NotificationChannel(id string, key []byte) (notify.Channel, notify.Credentials, error) {
	if err := s.ensureNotificationSchema(); err != nil {
		return notify.Channel{}, notify.Credentials{}, err
	}
	row := s.db.QueryRow(`SELECT id,type,label,target,enabled,credentials_ciphertext,created_at,updated_at FROM notification_channels WHERE id=?`, id)
	value, ciphertext, err := scanNotificationChannel(row)
	if err != nil {
		return notify.Channel{}, notify.Credentials{}, err
	}
	if len(ciphertext) == 0 {
		return value, notify.Credentials{}, nil
	}
	credentials, err := notify.DecryptCredentials(ciphertext, key)
	return value, credentials, err
}

func (s *Store) SaveNotificationChannel(value notify.Channel, credentials notify.Credentials, key []byte) (notify.Channel, error) {
	if err := value.Validate(); err != nil {
		return notify.Channel{}, err
	}
	if err := s.ensureNotificationSchema(); err != nil {
		return notify.Channel{}, err
	}
	ciphertext := []byte(nil)
	var err error
	if notify.HasCredentials(credentials) {
		ciphertext, err = notify.EncryptCredentials(credentials, key)
		if err != nil {
			return notify.Channel{}, err
		}
	}
	now := time.Now().UTC()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	value.Configured = len(ciphertext) > 0
	_, err = s.db.Exec(`INSERT INTO notification_channels(id,type,label,target,enabled,credentials_ciphertext,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET type=excluded.type,label=excluded.label,target=excluded.target,enabled=excluded.enabled,credentials_ciphertext=excluded.credentials_ciphertext,updated_at=excluded.updated_at`, value.ID, value.Type, value.Label, value.Target, value.Enabled, ciphertext, value.CreatedAt.Format(timeFormat), value.UpdatedAt.Format(timeFormat))
	return value, err
}

func (s *Store) DeleteNotificationChannel(id string) error {
	if err := s.ensureNotificationSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM notification_channels WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SaveNotificationDelivery(value notify.Delivery) error {
	if value.ID == "" || value.ChannelID == "" || value.EventType == "" || value.State == "" {
		return sql.ErrNoRows
	}
	if err := s.ensureNotificationSchema(); err != nil {
		return err
	}
	if value.AttemptedAt.IsZero() {
		value.AttemptedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO notification_deliveries(id,channel_id,event_type,state,attempted_at,error) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,attempted_at=excluded.attempted_at,error=excluded.error`, value.ID, value.ChannelID, value.EventType, value.State, value.AttemptedAt.Format(timeFormat), nullable(value.Error))
	return err
}

func (s *Store) NotificationDeliveries(limit int) ([]notify.Delivery, error) {
	if err := s.ensureNotificationSchema(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,channel_id,event_type,state,attempted_at,COALESCE(error,'') FROM notification_deliveries ORDER BY attempted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]notify.Delivery, 0)
	for rows.Next() {
		var value notify.Delivery
		var attempted string
		if err := rows.Scan(&value.ID, &value.ChannelID, &value.EventType, &value.State, &attempted, &value.Error); err != nil {
			return nil, err
		}
		value.AttemptedAt, err = time.Parse(timeFormat, attempted)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type notificationScanner interface {
	Scan(...any) error
}

func scanNotificationChannel(scanner notificationScanner) (notify.Channel, []byte, error) {
	var value notify.Channel
	var enabled int
	var ciphertext []byte
	var created, updated string
	if err := scanner.Scan(&value.ID, &value.Type, &value.Label, &value.Target, &enabled, &ciphertext, &created, &updated); err != nil {
		return notify.Channel{}, nil, err
	}
	value.Enabled = enabled != 0
	value.Configured = len(ciphertext) > 0
	var err error
	value.CreatedAt, err = time.Parse(timeFormat, created)
	if err != nil {
		return notify.Channel{}, nil, err
	}
	value.UpdatedAt, err = time.Parse(timeFormat, updated)
	return value, ciphertext, err
}
