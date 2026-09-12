package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

var sensitiveText = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|private[_-]?key|recovery[_-]?key)([[:space:]]*[:=][[:space:]]*)("[^"]*"|'[^']*'|[^[:space:],}]+)`)
var bearerText = regexp.MustCompile(`(?i)(bearer[[:space:]]+)[A-Za-z0-9._~+/=-]+`)
var privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)

func RedactText(value string) string {
	value = privateKeyBlock.ReplaceAllString(value, "[REDACTED]")
	value = sensitiveText.ReplaceAllString(value, `${1}${2}"[REDACTED]"`)
	return bearerText.ReplaceAllString(value, `${1}[REDACTED]`)
}

func RedactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = RedactValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = RedactValue(item)
		}
		return result
	case string:
		return RedactText(typed)
	default:
		return value
	}
}

func MarshalJSON(value any) ([]byte, error) {
	return json.MarshalIndent(RedactValue(value), "", "  ")
}

func CreateBundle(entries map[string][]byte) ([]byte, error) {
	if len(entries) == 0 {
		return nil, errors.New("support bundle has no entries")
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		if !safeName(name) {
			return nil, fmt.Errorf("unsafe support entry %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		content := entries[name]
		if sensitiveKey(path.Base(name)) {
			content = []byte("[REDACTED]")
		} else {
			content = []byte(RedactText(string(content)))
		}
		if _, err := entry.Write(content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func safeName(value string) bool {
	clean := path.Clean(value)
	return value != "" && clean == value && clean != "." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/") && !strings.Contains(value, "\\")
}

func sensitiveKey(value string) bool {
	lower := strings.ToLower(value)
	for _, separator := range []string{"-", "_", ".", "/"} {
		lower = strings.ReplaceAll(lower, separator, "")
	}
	for _, marker := range []string{"password", "passwd", "secret", "token", "apikey", "privatekey", "recoverykey", "authorization"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
