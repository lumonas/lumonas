package store

import (
	"database/sql"
	"errors"
	"time"
)

var ErrLanHostNotFound = errors.New("LAN host not found")

const lanHostsSchema = `
CREATE TABLE IF NOT EXISTS lan_hosts (
  mac TEXT NOT NULL,
  interface TEXT NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  hostname TEXT NOT NULL DEFAULT '',
  first_seen TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  PRIMARY KEY(mac, interface)
);`

func (s *Store) ensureLanHostsSchema() error {
	if _, err := s.db.Exec(lanHostsSchema); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(12, ?)`, time.Now().UTC().Format(timeFormat))
	return err
}

// LanHostRecord is one discovered neighbor, keyed by MAC and interface so a
// host seen on two segments stays two rows.
type LanHostRecord struct {
	MAC       string    `json:"mac"`
	Interface string    `json:"interface"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname,omitempty"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// UpsertLanHosts records an observation batch. Operator-set hostnames and
// the original first_seen survive every scan; only location and recency are
// refreshed.
func (s *Store) UpsertLanHosts(records []LanHostRecord) error {
	if err := s.ensureLanHostsSchema(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(timeFormat)
	for _, record := range records {
		if _, err := s.db.Exec(`INSERT INTO lan_hosts(mac,interface,ip,hostname,first_seen,last_seen) VALUES(?,?,?,'',?,?)
ON CONFLICT(mac,interface) DO UPDATE SET ip=excluded.ip,last_seen=excluded.last_seen`,
			record.MAC, record.Interface, record.IP, now, now); err != nil {
			return err
		}
	}
	return nil
}

// LanHosts lists every known neighbor, most recently seen first.
func (s *Store) LanHosts() ([]LanHostRecord, error) {
	if err := s.ensureLanHostsSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT mac,interface,COALESCE(ip,''),COALESCE(hostname,''),first_seen,last_seen FROM lan_hosts ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]LanHostRecord, 0)
	for rows.Next() {
		var record LanHostRecord
		var first, last string
		if err := rows.Scan(&record.MAC, &record.Interface, &record.IP, &record.Hostname, &first, &last); err != nil {
			return nil, err
		}
		record.FirstSeen, _ = parseTime(first)
		record.LastSeen, _ = parseTime(last)
		result = append(result, record)
	}
	return result, rows.Err()
}

// RenameLanHost stores an operator-provided hostname for a neighbor.
func (s *Store) RenameLanHost(mac, iface, hostname string) error {
	if err := s.ensureLanHostsSchema(); err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE lan_hosts SET hostname=? WHERE mac=? AND interface=?`, hostname, mac, iface)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrLanHostNotFound
	}
	return nil
}

// LanHostKnown reports whether a target was observed on the requested
// interface. Mutating LAN actions use this check so a caller cannot turn the
// inventory endpoint into an arbitrary packet-sending primitive.
func (s *Store) LanHostKnown(mac, iface string) (bool, error) {
	if err := s.ensureLanHostsSchema(); err != nil {
		return false, err
	}
	var present int
	err := s.db.QueryRow(`SELECT 1 FROM lan_hosts WHERE mac=? AND interface=? LIMIT 1`, mac, iface).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && present == 1, err
}
