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
        `CREATE TABLE IF NOT EXISTS admin (id INTEGER PRIMARY KEY, username TEXT UNIQUE, password_hash TEXT);`,
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
    }
    for _, stmt := range stmts {
        if _, err := s.DB.Exec(stmt); err != nil {
            return err
        }
    }
    return nil
}

func (s *Store) UpsertAdmin(username, passwordHash string) error {
    _, err := s.DB.Exec(`INSERT INTO admin (id, username, password_hash) VALUES (1, ?, ?) 
        ON CONFLICT(id) DO UPDATE SET username=excluded.username, password_hash=excluded.password_hash`, username, passwordHash)
    return err
}

func (s *Store) GetAdmin() (string, string, error) {
    row := s.DB.QueryRow(`SELECT username, password_hash FROM admin WHERE id=1`)
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

func (s *Store) DeactivateUser(id int64) error {
    res, err := s.DB.Exec(`UPDATE users SET active=0 WHERE id=?`, id)
    if err != nil {
        return err
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return errors.New("user not found")
    }
    return nil
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
