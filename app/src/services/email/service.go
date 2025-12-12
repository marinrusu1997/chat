package email

import (
	"crypto/tls"
)

// @FIXME add validations everywhere
// @FIXME get rid of pointers where not needed

type Service struct {
}

type ServiceSMTPOptions struct {
	Address   string
	TLSConfig *tls.Config
	Username  string
	Password  string
}

type ServiceOptions struct {
	SMTP ServiceSMTPOptions
}
