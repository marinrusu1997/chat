package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	netmail "net/mail"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/rs/zerolog"
	"github.com/wneessen/go-mail"
)

var ErrSendEmailInvalidSenderCount = errors.New("email can't be sent because it has invalid sender count")
var ErrSendEmailInvalidReceiverCount = errors.New("email can't be sent because it has no receivers")

type smtpClient struct {
	driver *smtp.Client
	opts   *SMTPClientOptions
}

type SMTPClientOptions struct {
	Host             string
	Port             uint16
	TLSConfig        *tls.Config
	Auth             sasl.Client
	ReconnectTimeout time.Duration
	Logger           *zerolog.Logger
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
	if err := client.Auth(c.opts.Auth); err != nil {
		tlsStatus := rollback(tlsConn)
		tcpStatus := rollback(tcpConn)
		return fmt.Errorf(
			"failed authentication to '%s' (rollback: TLS connection status '%s', TPC connection status '%s'): %w",
			address, tlsStatus, tcpStatus, err,
		)
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
	done := make(chan error, 1)

	go func() {
		done <- c.SendEmailNoContext(opts)
	}()

	select {
	case err := <-done:
		return err

	case <-ctx.Done():
		return fmt.Errorf("context is done: %w", ctx.Err())
	}
}

func (c *smtpClient) SendEmailNoContext(opts SendEmailOptions) error {
	// From
	senders := opts.Email.GetFrom()
	if len(senders) != 1 {
		return fmt.Errorf("expected exactly one sender, got %d: %w", len(senders), ErrSendEmailInvalidSenderCount)
	}
	if err := c.driver.Mail(senders[0].Address, opts.SendOptions); err != nil {
		return fmt.Errorf("MAIL FROM '%s' failed: %w", senders[0].Address, err)
	}

	// To
	{
		toCount, err := c.rcpt(opts.Email.GetTo(), opts.ReceiveOptions)
		if err != nil {
			return err //nolint:wrapcheck // we are good here
		}

		ccCount, err := c.rcpt(opts.Email.GetCc(), opts.ReceiveOptions)
		if err != nil {
			return err //nolint:wrapcheck // we are good here
		}

		bccCount, err := c.rcpt(opts.Email.GetBcc(), opts.ReceiveOptions)
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

func (c *smtpClient) rcpt(addresses []*netmail.Address, opts *smtp.RcptOptions) (int, error) {
	for _, address := range addresses {
		if err := c.driver.Rcpt(address.Address, opts); err != nil {
			if err := c.driver.Reset(); err != nil {
				c.opts.Logger.Warn().Err(err).Msgf("failed to reset SMTP client after RCPT TO '%s' failure", address)
			}
			return 0, fmt.Errorf("RCPT TO '%s' failed: %w", address.Address, err)
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
