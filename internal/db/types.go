package db

import (
	"database/sql/driver"
	"encoding/json"
)

// StringArray is a []string that works with PostgreSQL text[] columns.
// Uses JSON marshaling for DB scan/value — no pq dependency needed.
type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return "{}", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "{}", nil
	}
	// Convert JSON array to PostgreSQL array format
	// JSON: ["a","b"] → PG: {a,b}
	result := string(b)
	result = `"` + result[1:len(result)-1] + `"` // strip brackets
	// Actually, just use lib/pq format
	return pqFormat(s), nil
}

func (s *StringArray) Scan(src interface{}) error {
	if src == nil {
		*s = []string{}
		return nil
	}
	switch v := src.(type) {
	case []byte:
		return s.parsePG(string(v))
	case string:
		return s.parsePG(v)
	}
	*s = []string{}
	return nil
}

func (s *StringArray) parsePG(str string) error {
	if str == "{}" || str == "" || str == "NULL" {
		*s = []string{}
		return nil
	}
	// Simple PG array parser: {item1,item2,"item with spaces"}
	str = str[1 : len(str)-1] // strip { }
	var result []string
	current := ""
	inQuote := false
	for _, c := range str {
		switch c {
		case '"':
			inQuote = !inQuote
		case ',':
			if !inQuote {
				result = append(result, current)
				current = ""
			} else {
				current += string(c)
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	*s = result
	return nil
}

func pqFormat(s []string) string {
	if len(s) == 0 {
		return "{}"
	}
	result := "{"
	for i, item := range s {
		if i > 0 {
			result += ","
		}
		// Escape quotes and backslashes
		escaped := stringsReplaceAll(item, `\`, `\\`)
		escaped = stringsReplaceAll(escaped, `"`, `\"`)
		result += `"` + escaped + `"`
	}
	result += "}"
	return result
}

func stringsReplaceAll(s, old, new string) string {
	// Simple replace all since we can't import strings in this file easily
	result := ""
	for i := 0; i < len(s); i++ {
		if i <= len(s)-len(old) && s[i:i+len(old)] == old {
			result += new
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}
