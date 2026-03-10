package logging

import (
    "log"
    "os"
)

func Setup(path string) (*os.File, error) {
    if path == "" {
        return nil, nil
    }
    if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
        return nil, err
    }
    f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil {
        return nil, err
    }
    log.SetOutput(f)
    return f, nil
}

func filepathDir(p string) string {
    for i := len(p) - 1; i >= 0; i-- {
        if p[i] == '/' || p[i] == '\\' {
            return p[:i]
        }
    }
    return "."
}
