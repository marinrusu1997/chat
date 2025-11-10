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

func (s Secret) String() string { //nolint:revive // we need to implement this interface
	return SecretMarker
}

func (s Secret) GoString() string { //nolint:revive // we need to implement this interface
	return SecretMarker
}

func (s Secret) Format(f fmt.State, _ rune) { //nolint:revive // we need to implement this interface
	_, err := f.Write([]byte(SecretMarker))
	if err != nil {
		return
	}
}

func (s Secret) LogValue() slog.Value { //nolint:forbidigo,revive // we need to implement this interface
	return slog.StringValue(SecretMarker) //nolint:forbidigo // we need to implement this interface
}

func (s Secret) MarshalText() ([]byte, error) { //nolint:unparam,revive // we need to implement this interface
	return []byte(SecretMarker), nil
}

func (s Secret) MarshalJSON() ([]byte, error) { //nolint:unparam,revive // we need to implement this interface
	return []byte(SecretMarker), nil
}

func (s Secret) MarshalYAML() (any, error) { //nolint:revive // we need to implement this interface
	return SecretMarker, nil
}

func (s Secret) MarshalZerologObject(e *zerolog.Event) { //nolint:revive // we need to implement this interface
	e.Str("secret", SecretMarker)
}
