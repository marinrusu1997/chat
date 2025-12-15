package state

import (
	"chat/src/services/email"
	"chat/src/services/presence"
)

type Services struct {
	Presence *presence.Service
	Email    *email.Service
}
