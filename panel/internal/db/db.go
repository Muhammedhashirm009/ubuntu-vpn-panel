package db

import (
    "database/sql"
    "errors"
    "time"

    _ "github.com/mattn/go-sqlite3"
)

type Store struct {
    DB *sql.DB
}

type User struct {
    ID        int64     `json:"id"`
    Protocol  string    `json:"protocol"`
    Username  string    `json:"username"`
    Password  string    `json:"password,omitempty"`
    UUID      string    `json:"uuid,omitempty"`
    Remark    string    `json:"remark,omitempty"`
    ExpiresAt time.Time `json:"expires_at"`
    Active    bool      `json:"active"`
    CreatedAt time.Time `json:"created_at"`
    Link      string    `json:"link,omitempty"`
}

type Audit struct {
    ID        int64     `json:"id"`
    Action    string    `json:"action"`
    Detail    string    `json:"detail"`
    CreatedAt time.Time `json:"created_at"`
}

func New(path string) (*Store, error) {
    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, err
    }
    s := &Store{DB: db}
    if err := s.migrate(); err != nil {
        return nil, err
    }
    return s, nil
}

func (s *Store) migrate() error {
    stmts := []string{
        `CREATE TABLE IF NOT EXISTS admin (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE, password_hash TEXT);`,
        `CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT);`,
        `CREATE TABLE IF NOT EXISTS private_dns (id INTEGER PRIMARY KEY AUTOINCREMENT, domain TEXT UNIQUE, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
        `CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            protocol TEXT,
            username TEXT,
            password TEXT,
            uuid TEXT,
            remark TEXT,
            expires_at DATETIME,
            active BOOLEAN DEFAULT 1,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );`,
        `CREATE TABLE IF NOT EXISTS audits (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            action TEXT,
            detail TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );`,
        `CREATE TABLE IF NOT EXISTS network_stats (
            date TEXT PRIMARY KEY,
            rx_bytes INTEGER DEFAULT 0,
            tx_bytes INTEGER DEFAULT 0
        );`,
    }
    for _, stmt := range stmts {
        if _, err := s.DB.Exec(stmt); err != nil {
            return err
        }
    }
    
    // Default settings if empty
    s.DB.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('setup_complete', 'false')`)
    s.DB.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('vpn_domain', '')`)
    
    return nil
}

func (s *Store) GetSetting(key string) string {
    var val string
    err := s.DB.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&val)
    if err != nil {
        return ""
    }
    return val
}

func (s *Store) SetSetting(key, value string) error {
    _, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
    return err
}

func (s *Store) AddAdmin(username, passwordHash string) error {
    _, err := s.DB.Exec(`INSERT INTO admin (username, password_hash) VALUES (?, ?)`, username, passwordHash)
    return err
}

func (s *Store) UpdateAdminPassword(username, passwordHash string) error {
    _, err := s.DB.Exec(`UPDATE admin SET password_hash=? WHERE username=?`, passwordHash, username)
    return err
}

func (s *Store) DeleteAdmin(id int64) error {
    _, err := s.DB.Exec(`DELETE FROM admin WHERE id=? AND id != 1`, id) // Prevent deleting the primary admin
    return err
}

type AdminUser struct {
    ID       int64  `json:"id"`
    Username string `json:"username"`
}

func (s *Store) ListAdmins() ([]AdminUser, error) {
    rows, err := s.DB.Query(`SELECT id, username FROM admin ORDER BY id ASC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []AdminUser
    for rows.Next() {
        var a AdminUser
        if err := rows.Scan(&a.ID, &a.Username); err != nil {
            return nil, err
        }
        out = append(out, a)
    }
    return out, nil
}

func (s *Store) GetAdmin(username string) (string, string, error) {
    row := s.DB.QueryRow(`SELECT username, password_hash FROM admin WHERE username=?`, username)
    var user, hash string
    if err := row.Scan(&user, &hash); err != nil {
        return "", "", err
    }
    return user, hash, nil
}

func (s *Store) GetFirstAdmin() (string, string, error) {
    row := s.DB.QueryRow(`SELECT username, password_hash FROM admin ORDER BY id ASC LIMIT 1`)
    var user, hash string
    if err := row.Scan(&user, &hash); err != nil {
        return "", "", err
    }
    return user, hash, nil
}

func (s *Store) AddUser(u User) (int64, error) {
    res, err := s.DB.Exec(`INSERT INTO users (protocol, username, password, uuid, remark, expires_at, active) VALUES (?,?,?,?,?,?,?)`,
        u.Protocol, u.Username, u.Password, u.UUID, u.Remark, u.ExpiresAt, u.Active)
    if err != nil {
        return 0, err
    }
    return res.LastInsertId()
}

func (s *Store) ListUsers() ([]User, error) {
    rows, err := s.DB.Query(`SELECT id, protocol, username, password, uuid, remark, expires_at, active, created_at FROM users ORDER BY created_at DESC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []User
    for rows.Next() {
        var u User
        if err := rows.Scan(&u.ID, &u.Protocol, &u.Username, &u.Password, &u.UUID, &u.Remark, &u.ExpiresAt, &u.Active, &u.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, u)
    }
    return out, nil
}

// Hard Delete entirely wipes the user from SQLite instead of deactivating
func (s *Store) HardDeleteUser(id int64) error {
    res, err := s.DB.Exec(`DELETE FROM users WHERE id=?`, id)
    if err != nil {
        return err
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return errors.New("user not found")
    }
    return nil
}

type PrivateDNS struct {
    ID        int64     `json:"id"`
    Domain    string    `json:"domain"`
    CreatedAt time.Time `json:"created_at"`
}

func (s *Store) AddDNSDomain(domain string) (int64, error) {
    res, err := s.DB.Exec(`INSERT INTO private_dns (domain) VALUES (?)`, domain)
    if err != nil {
        return 0, err
    }
    return res.LastInsertId()
}

func (s *Store) ListDNSDomains() ([]PrivateDNS, error) {
    rows, err := s.DB.Query(`SELECT id, domain, created_at FROM private_dns ORDER BY created_at DESC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []PrivateDNS
    for rows.Next() {
        var d PrivateDNS
        if err := rows.Scan(&d.ID, &d.Domain, &d.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, d)
    }
    return out, nil
}

func (s *Store) DeleteDNSDomain(id int64) error {
    _, err := s.DB.Exec(`DELETE FROM private_dns WHERE id=?`, id)
    return err
}

func (s *Store) AddAudit(action, detail string) error {
    _, err := s.DB.Exec(`INSERT INTO audits (action, detail) VALUES (?, ?)`, action, detail)
    return err
}

func (s *Store) ListAudits(limit int) ([]Audit, error) {
    rows, err := s.DB.Query(`SELECT id, action, detail, created_at FROM audits ORDER BY created_at DESC LIMIT ?`, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []Audit
    for rows.Next() {
        var a Audit
        if err := rows.Scan(&a.ID, &a.Action, &a.Detail, &a.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, a)
    }
    return out, nil
}

type NetworkStat struct {
    Date    string `json:"date"`
    RxBytes uint64 `json:"rx_bytes"`
    TxBytes uint64 `json:"tx_bytes"`
}

// Upsert traffic (add delta to existing row)
func (s *Store) AddNetworkUsage(date string, rxDelta, txDelta uint64) error {
    _, err := s.DB.Exec(`
        INSERT INTO network_stats (date, rx_bytes, tx_bytes) 
        VALUES (?, ?, ?) 
        ON CONFLICT(date) DO UPDATE SET 
            rx_bytes = rx_bytes + excluded.rx_bytes,
            tx_bytes = tx_bytes + excluded.tx_bytes
    `, date, rxDelta, txDelta)
    return err
}

func (s *Store) GetNetworkUsageToday(date string) (rx, tx uint64) {
    s.DB.QueryRow(`SELECT rx_bytes, tx_bytes FROM network_stats WHERE date = ?`, date).Scan(&rx, &tx)
    return
}

func (s *Store) GetNetworkUsageMonth(monthPrefix string) (rx, tx uint64) {
    s.DB.QueryRow(`SELECT SUM(rx_bytes), SUM(tx_bytes) FROM network_stats WHERE date LIKE ?`, monthPrefix+"%").Scan(&rx, &tx)
    return
}

func (s *Store) GetNetworkUsageTotal() (rx, tx uint64) {
    s.DB.QueryRow(`SELECT SUM(rx_bytes), SUM(tx_bytes) FROM network_stats`).Scan(&rx, &tx)
    return
}
