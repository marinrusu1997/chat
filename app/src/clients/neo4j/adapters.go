package neo4j

import (
	"fmt"

	"github.com/rs/zerolog"
)

// driverLoggerAdapter adapts zerolog to the neo4j.DriverLogger interface.
type driverLoggerAdapter struct {
	logger zerolog.Logger
}

func (a *driverLoggerAdapter) Error(name, id string, err error) {
	a.logger.Error().Err(err).Str("name", name).Str("id", id).Msg("Neo4j Driver Error")
}
func (a *driverLoggerAdapter) Warnf(name, id, msg string, args ...any) {
	a.logger.Warn().Str("name", name).Str("id", id).Msg(fmt.Sprintf(msg, args...))
}
func (a *driverLoggerAdapter) Infof(name, id, msg string, args ...any) {
	a.logger.Info().Str("name", name).Str("id", id).Msg(fmt.Sprintf(msg, args...))
}
func (a *driverLoggerAdapter) Debugf(name, id, msg string, args ...any) {
	a.logger.Debug().Str("name", name).Str("id", id).Msg(fmt.Sprintf(msg, args...))
}

// sessionLoggerAdapter adapts zerolog to the neo4j.SessionLogger interface.
type sessionLoggerAdapter struct {
	logger zerolog.Logger
}

func (a *sessionLoggerAdapter) LogClientMessage(id, msg string, args ...any) {
	a.logger.Info().Str("id", id).Str("source", "client").Msgf(msg, args...)
}

func (a *sessionLoggerAdapter) LogServerMessage(id, msg string, args ...any) {
	a.logger.Info().Str("id", id).Str("source", "server").Msgf(msg, args...)
}
