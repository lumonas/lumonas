package store

import (
	"database/sql"
	"errors"
	"time"
)

// PasskeyCredential is a registered WebAuthn credential for a management user.
type PasskeyCredential struct {
	ID                 []byte    `json:"id"`
	PublicKey          []byte    `json:"publicKey"`
	Attestation        []byte    `json:"attestation"`
	ClientDataJSON     []byte    `json:"clientDataJson,omitempty"`
	AuthenticatorData  []byte    `json:"authenticatorData,omitempty"`
	PublicKeyAlgorithm int64     `json:"publicKeyAlgorithm,omitempty"`
	AttestationType    string    `json:"attestationType,omitempty"`
	AttestationFormat  string    `json:"attestationFormat,omitempty"`
	UserID             string    `json:"userId"`
	SignCount          uint32    `json:"signCount"`
	CloneWarning       bool      `json:"-"`
	Name               string    `json:"name"`
	CreatedAt          time.Time `json:"createdAt"`
}

// ErrPasskeyNotFound reports an unknown or already-deleted credential.
var ErrPasskeyNotFound = errors.New("passkey credential not found")

func (s *Store) ensurePasskeysSchema() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS passkey_credentials(
		id BLOB PRIMARY KEY,
		public_key BLOB NOT NULL,
		attestation BLOB,
		client_data_json BLOB,
		authenticator_data BLOB,
		public_key_algorithm INTEGER NOT NULL DEFAULT 0,
		attestation_type TEXT NOT NULL DEFAULT '',
		attestation_format TEXT NOT NULL DEFAULT '',
		user_id TEXT NOT NULL,
		sign_count INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	columns := map[string]string{
		"attestation":          "BLOB",
		"client_data_json":     "BLOB",
		"authenticator_data":   "BLOB",
		"public_key_algorithm": "INTEGER NOT NULL DEFAULT 0",
		"attestation_type":     "TEXT NOT NULL DEFAULT ''",
		"attestation_format":   "TEXT NOT NULL DEFAULT ''",
	}
	rows, err := s.db.Query(`PRAGMA table_info(passkey_credentials)`)
	if err != nil {
		return err
	}
	present := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		present[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for name, definition := range columns {
		if present[name] {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE passkey_credentials ADD COLUMN ` + name + ` ` + definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SavePasskeyCredential(credential PasskeyCredential) error {
	if err := s.ensurePasskeysSchema(); err != nil {
		return err
	}
	if len(credential.ID) == 0 || len(credential.PublicKey) == 0 || credential.UserID == "" {
		return errors.New("passkey credential requires id, public key, and user")
	}
	if credential.CreatedAt.IsZero() {
		credential.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO passkey_credentials(id,public_key,attestation,client_data_json,authenticator_data,public_key_algorithm,attestation_type,attestation_format,user_id,sign_count,name,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET public_key=excluded.public_key,attestation=excluded.attestation,client_data_json=excluded.client_data_json,authenticator_data=excluded.authenticator_data,public_key_algorithm=excluded.public_key_algorithm,attestation_type=excluded.attestation_type,attestation_format=excluded.attestation_format,user_id=excluded.user_id,sign_count=excluded.sign_count,name=excluded.name`,
		credential.ID, credential.PublicKey, credential.Attestation, credential.ClientDataJSON, credential.AuthenticatorData, credential.PublicKeyAlgorithm, credential.AttestationType, credential.AttestationFormat, credential.UserID, credential.SignCount, credential.Name, credential.CreatedAt.Format(timeFormat))
	return err
}

func (s *Store) PasskeyCredential(id []byte) (PasskeyCredential, error) {
	if err := s.ensurePasskeysSchema(); err != nil {
		return PasskeyCredential{}, err
	}
	var credential PasskeyCredential
	var created string
	err := s.db.QueryRow(`SELECT id,public_key,attestation,client_data_json,authenticator_data,public_key_algorithm,attestation_type,attestation_format,user_id,sign_count,name,created_at FROM passkey_credentials WHERE id=?`, id).
		Scan(&credential.ID, &credential.PublicKey, &credential.Attestation, &credential.ClientDataJSON, &credential.AuthenticatorData, &credential.PublicKeyAlgorithm, &credential.AttestationType, &credential.AttestationFormat, &credential.UserID, &credential.SignCount, &credential.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return PasskeyCredential{}, ErrPasskeyNotFound
	}
	if err != nil {
		return PasskeyCredential{}, err
	}
	credential.CreatedAt, _ = parseTime(created)
	return credential, nil
}

func (s *Store) PasskeyCredentials(userID string) ([]PasskeyCredential, error) {
	if err := s.ensurePasskeysSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,public_key,attestation,client_data_json,authenticator_data,public_key_algorithm,attestation_type,attestation_format,user_id,sign_count,name,created_at FROM passkey_credentials WHERE user_id=? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PasskeyCredential, 0)
	for rows.Next() {
		var credential PasskeyCredential
		var created string
		if err := rows.Scan(&credential.ID, &credential.PublicKey, &credential.Attestation, &credential.ClientDataJSON, &credential.AuthenticatorData, &credential.PublicKeyAlgorithm, &credential.AttestationType, &credential.AttestationFormat, &credential.UserID, &credential.SignCount, &credential.Name, &created); err != nil {
			return nil, err
		}
		credential.CreatedAt, _ = parseTime(created)
		result = append(result, credential)
	}
	return result, rows.Err()
}

func (s *Store) DeletePasskeyCredential(id []byte, userID string) error {
	if err := s.ensurePasskeysSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM passkey_credentials WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrPasskeyNotFound
	}
	return nil
}

// Passkey sign counts must advance monotonically; the update rejects stale
// counters which indicate a cloned authenticator.
func (s *Store) UpdatePasskeySignCount(id []byte, signCount uint32) error {
	if err := s.ensurePasskeysSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE passkey_credentials SET sign_count=? WHERE id=? AND sign_count < ?`, signCount, id, signCount)
	return err
}
