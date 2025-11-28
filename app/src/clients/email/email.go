package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	netmail "net/mail"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/rs/zerolog"
	"github.com/wneessen/go-mail"
)

var ErrSendEmailInvalidSenderCount = errors.New("email can't be sent because it has invalid sender count")
var ErrSendEmailInvalidReceiverCount = errors.New("email can't be sent because it has no receivers")

var smtpExtensions = []string{
	"PIPELINING",
	"CHUNKING",
	"8BITMIME",
	"BINARYMIME",
	"UTF8SMTP",
	"SMTPUTF8",
	"MT-PRIORITY",
	"SIZE",
	"DSN",
	"MTRK",
}
var smtpCapabilitiesLogged sync.Map // map[string]struct{}

type smtpClient struct {
	driver *smtp.Client
	opts   *SMTPClientOptions
}

type SMTPClientOptions struct {
	Host              string
	Port              uint16
	TLSConfig         *tls.Config
	Auth              sasl.Client
	ReconnectTimeout  time.Duration
	CommandTimeout    time.Duration
	SubmissionTimeout time.Duration
	SendTimeout       time.Duration
	Logger            *zerolog.Logger
}

type SendEmailOptions struct {
	Email          *mail.Msg
	SendOptions    *smtp.MailOptions
	ReceiveOptions *smtp.RcptOptions
}

func newSMTPClient(opts *SMTPClientOptions) *smtpClient {
	return &smtpClient{
		driver: nil,
		opts:   opts,
	}
}

func (c *smtpClient) Connect(ctx context.Context) error {
	if c.driver != nil {
		return nil
	}

	var dialer net.Dialer

	address := fmt.Sprintf("%s:%d", c.opts.Host, c.opts.Port)
	tcpConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("failed tcp dial to '%s': %w", address, err)
	}

	tlsConn := tls.Client(tcpConn, c.opts.TLSConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		status := rollback(tcpConn)
		return fmt.Errorf(
			"failed tls handshake to '%s' (rollback TCP connection status '%s'): %w",
			address, status, err,
		)
	}

	client := smtp.NewClient(tlsConn)
	client.SubmissionTimeout = c.opts.SubmissionTimeout
	client.CommandTimeout = c.opts.CommandTimeout

	if err := client.Auth(c.opts.Auth); err != nil {
		tlsStatus := rollback(tlsConn)
		tcpStatus := rollback(tcpConn)
		return fmt.Errorf(
			"failed authentication to '%s' (rollback: TLS connection status '%s', TPC connection status '%s'): %w",
			address, tlsStatus, tcpStatus, err,
		)
	}

	{
		_, loaded := smtpCapabilitiesLogged.LoadOrStore(address, struct{}{})
		if !loaded {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("SMTP server '%s' capabilities:\n", address)) //nolint:errcheck // it returns nil error
			for _, ext := range smtpExtensions {
				supported, args := client.Extension(ext)

				if supported {
					if args != "" {
						sb.WriteString(fmt.Sprintf("- %-10s\tYES\t%s\n", ext, args)) //nolint:errcheck // it returns nil error
					} else {
						sb.WriteString(fmt.Sprintf("- %-10s\tYES\n", ext)) //nolint:errcheck // it returns nil error
					}
				} else {
					sb.WriteString(fmt.Sprintf("- %-10s\tNO\n", ext)) //nolint:errcheck // it returns nil error
				}
			}
			c.opts.Logger.Info().Msg(sb.String())
		}
	}

	c.driver = client
	return nil
}

func (c *smtpClient) Disconnect() error {
	if c.driver == nil {
		return nil
	}

	driver := c.driver
	c.driver = nil

	if err := driver.Close(); err != nil {
		return fmt.Errorf("failed to close SMTP client: %w", err)
	}
	return nil
}

func (c *smtpClient) SendEmail(ctx context.Context, opts SendEmailOptions) error {
	// From
	senders := opts.Email.GetFrom()
	if len(senders) != 1 {
		return fmt.Errorf("expected exactly one sender, got %d: %w", len(senders), ErrSendEmailInvalidSenderCount)
	}
	if err := c.driver.Mail(senders[0].Address, opts.SendOptions); err != nil {
		return fmt.Errorf("MAIL FROM '%s' failed: %w", senders[0].Address, err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("MAIL FROM '%s' timedout: %w", senders[0].Address, ctx.Err())
	default:
		// continue
	}

	// To
	{
		toCount, err := c.rcpt(ctx, opts.Email.GetTo(), opts.ReceiveOptions)
		if err != nil {
			return err //nolint:wrapcheck // we are good here
		}

		ccCount, err := c.rcpt(ctx, opts.Email.GetCc(), opts.ReceiveOptions)
		if err != nil {
			return err //nolint:wrapcheck // we are good here
		}

		bccCount, err := c.rcpt(ctx, opts.Email.GetBcc(), opts.ReceiveOptions)
		if err != nil {
			return err //nolint:wrapcheck // we are good here
		}

		if toCount+ccCount+bccCount == 0 {
			return ErrSendEmailInvalidReceiverCount
		}
	}

	// Body
	dataCommand, err := c.driver.Data()
	if err != nil {
		c.reconnect() // connection state may be invalid, try to reconnect
		return fmt.Errorf("DATA failed: %w", err)
	}

	if _, err := opts.Email.WriteTo(dataCommand); err != nil {
		status := rollback(dataCommand)
		c.reconnect() // connection state may be invalid, try to reconnect
		return fmt.Errorf("failed to write email body (rollback DATA command status '%s'): %w", status, err)
	}

	if err := dataCommand.Close(); err != nil {
		c.reconnect() // connection state may be invalid, try to reconnect
		return fmt.Errorf("failed to close DATA command: %w", err)
	}

	// Done
	return nil
}

func (c *smtpClient) rcpt(ctx context.Context, addresses []*netmail.Address, opts *smtp.RcptOptions) (int, error) {
	for _, address := range addresses {
		if err := c.driver.Rcpt(address.Address, opts); err != nil {
			if err := c.driver.Reset(); err != nil {
				c.opts.Logger.Warn().Err(err).Msgf("failed to reset SMTP client after RCPT TO '%s' failure", address)
			}
			return 0, fmt.Errorf("RCPT TO '%s' failed: %w", address.Address, err)
		}

		select {
		case <-ctx.Done():
			if err := c.driver.Reset(); err != nil {
				c.opts.Logger.Warn().Err(err).Msgf("failed to reset SMTP client after RCPT TO '%s' timeout", address)
			}
			return 0, fmt.Errorf("RCPT TO '%s' timedout: %w", address.Address, ctx.Err())
		default:
			// continue
		}
	}

	return len(addresses), nil
}

func (c *smtpClient) reconnect() {
	if err := c.Disconnect(); err != nil {
		c.opts.Logger.Error().Err(err).Msg("failed to disconnect SMTP client during reconnect")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.opts.ReconnectTimeout)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		c.opts.Logger.Error().Err(err).Msg("failed to reconnect SMTP client")
	}
}

func rollback(closer io.Closer) string {
	msg := "success"
	if err := closer.Close(); err != nil {
		msg = fmt.Sprintf("failure: %v", err)
	}
	return msg
}
