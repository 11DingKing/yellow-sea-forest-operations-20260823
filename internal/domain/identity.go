package domain

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

type ID string

func NewID(prefix string) (ID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	prefix = strings.Trim(strings.ToLower(prefix), "- ")
	if prefix == "" {
		return ID(hex.EncodeToString(raw[:])), nil
	}
	return ID(prefix + "_" + hex.EncodeToString(raw[:])), nil
}

func ParseID(field, value string) (ID, error) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || len(value) > 96 {
		return "", FieldError{Field: field, Message: "must contain 3 to 96 characters"}
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "", FieldError{Field: field, Message: "contains unsupported characters"}
	}
	return ID(value), nil
}

func (id ID) String() string { return string(id) }

func (id ID) Valid() bool {
	_, err := ParseID("id", string(id))
	return err == nil
}
