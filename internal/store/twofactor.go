package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	totpPendingColumn  = "totp_pending"
	totpSecretColumn   = "totp_secret"
	totpEnabledColumn  = "totp_enabled"
	totpRecoveryColumn = "totp_recovery_json"
)

// ErrTOTPNotConfigured reports missing two-factor state.
var ErrTOTPNotConfigured = errors.New("two-factor authentication is not configured for this principal")

func decodeJSONStrings(encoded string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(encoded), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func encodeJSONStrings(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	return string(encoded), err
}

// ensureTOTPColumns adds the two-factor columns to the principals table when
// upgrading an existing appliance database.
func (s *Store) ensureTOTPColumns() error {
	rows, err := s.db.Query(`PRAGMA table_info(principals)`)
	if err != nil {
		return err
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	migrations := map[string]string{
		totpPendingColumn:  `ALTER TABLE principals ADD COLUMN totp_pending BLOB`,
		totpSecretColumn:   `ALTER TABLE principals ADD COLUMN totp_secret BLOB`,
		totpEnabledColumn:  `ALTER TABLE principals ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0`,
		totpRecoveryColumn: `ALTER TABLE principals ADD COLUMN totp_recovery_json TEXT`,
	}
	for column, statement := range migrations {
		if columns[column] {
			continue
		}
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("add column %s: %w", column, err)
		}
	}
	return nil
}

func (s *Store) TOTPEnabled(id string) (bool, error) {
	if err := s.ensureTOTPColumns(); err != nil {
		return false, err
	}
	var enabled int
	if err := s.db.QueryRow(`SELECT totp_enabled FROM principals WHERE id=?`, id).Scan(&enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrTOTPNotConfigured
		}
		return false, err
	}
	return enabled != 0, nil
}

// TOTPRecord loads the full two-factor state: pending ciphertext (enrolment in
// progress), active secret ciphertext, enabled flag, and recovery-code hashes.
func (s *Store) TOTPRecord(id string) (pending, secret []byte, enabled bool, recoveryHashes []string, err error) {
	if err = s.ensureTOTPColumns(); err != nil {
		return
	}
	var pendingBlob, secretBlob []byte
	var recovery sql.NullString
	var enabledInt int
	err = s.db.QueryRow(`SELECT totp_pending,totp_secret,totp_enabled,totp_recovery_json FROM principals WHERE id=?`, id).Scan(&pendingBlob, &secretBlob, &enabledInt, &recovery)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrTOTPNotConfigured
		}
		return
	}
	pending, secret = pendingBlob, secretBlob
	enabled = enabledInt != 0
	if recovery.Valid && recovery.String != "" {
		recoveryHashes, err = decodeJSONStrings(recovery.String)
	}
	return
}

// SaveTOTPPending stages a secret ciphertext and recovery hashes until the
// user confirms enrolment with a valid code.
func (s *Store) SaveTOTPPending(id string, secretCiphertext []byte, recoveryHashes []string) error {
	if err := s.ensureTOTPColumns(); err != nil {
		return err
	}
	encoded, err := encodeJSONStrings(recoveryHashes)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE principals SET totp_pending=?,totp_recovery_json=?,totp_enabled=0,updated_at=? WHERE id=?`, secretCiphertext, encoded, time.Now().UTC().Format(timeFormat), id)
	return err
}

// EnableTOTP promotes the pending secret to the active secret.
func (s *Store) EnableTOTP(id string, secretCiphertext []byte) error {
	if err := s.ensureTOTPColumns(); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE principals SET totp_secret=?,totp_pending=NULL,totp_enabled=1,updated_at=? WHERE id=?`, secretCiphertext, time.Now().UTC().Format(timeFormat), id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrTOTPNotConfigured
	}
	return nil
}

func (s *Store) DisableTOTP(id string) error {
	if err := s.ensureTOTPColumns(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE principals SET totp_secret=NULL,totp_pending=NULL,totp_enabled=0,totp_recovery_json=NULL,updated_at=? WHERE id=?`, time.Now().UTC().Format(timeFormat), id)
	return err
}

// ConsumeRecoveryCode removes a single-use recovery code hash; it reports
// whether the code was still present.
func (s *Store) ConsumeRecoveryCode(id, hash string) (bool, error) {
	_, _, _, hashes, err := s.TOTPRecord(id)
	if err != nil {
		return false, err
	}
	remaining := make([]string, 0, len(hashes))
	found := false
	for _, candidate := range hashes {
		if candidate == hash && !found {
			found = true
			continue
		}
		remaining = append(remaining, candidate)
	}
	if !found {
		return false, nil
	}
	if err := s.ensureTOTPColumns(); err != nil {
		return false, err
	}
	encoded, err := encodeJSONStrings(remaining)
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(`UPDATE principals SET totp_recovery_json=?,updated_at=? WHERE id=?`, encoded, time.Now().UTC().Format(timeFormat), id)
	return true, err
}
