package util

import (
	"fmt"
	"log/slog"

	"github.com/rs/zerolog"
)

const (
	SecretMarker = "************"
)

type Secret string

func (s Secret) String() string {
	return SecretMarker
}

func (s Secret) GoString() string {
	return SecretMarker
}

func (s Secret) Format(f fmt.State, _ rune) {
	_, err := f.Write([]byte(SecretMarker))
	if err != nil {
		return
	}
}

func (s Secret) LogValue() slog.Value {
	return slog.StringValue(SecretMarker)
}

func (s Secret) MarshalText() ([]byte, error) {
	return []byte(SecretMarker), nil
}

func (s Secret) MarshalJSON() ([]byte, error) {
	return []byte(SecretMarker), nil
}

func (s Secret) MarshalYAML() (interface{}, error) {
	return SecretMarker, nil
}

func (s Secret) MarshalZerologObject(e *zerolog.Event) {
	e.Str("secret", SecretMarker)
}
