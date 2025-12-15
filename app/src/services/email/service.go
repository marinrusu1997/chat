package email

import (
	"chat/src/clients/email"
	"chat/src/clients/kafka"
	"chat/src/clients/kafka/routing"
	emailv1 "chat/src/gen/proto/email/v1"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/textproto"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/emersion/go-smtp"
	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/wneessen/go-mail"
	"google.golang.org/protobuf/proto"
)

// @FIXME use schema registry for Proto messages
// @FIXME use DLQ, idempotency

var ErrInvalidEmailRequest = errors.New("invalid email request")

type clients struct {
	email *email.Client
	kafka *kafka.Client
}

type emailMsgBuildOpts struct {
	from         string
	organization string
	userAgent    string
	dkimCert     *tls.Certificate
}

type kafkaDeliveryOpts struct {
	topic  string
	router *routing.ConsumerRouter
}

type Service struct {
	clients       clients
	emailMsgBuild emailMsgBuildOpts
	kafkaDelivery kafkaDeliveryOpts
	logger        *zerolog.Logger
}

type ServiceEmailBuildOptions struct {
	From         string
	Organization string
	UserAgent    string
	DKIMCert     *tls.Certificate
}

type ServiceKafkaDeliveryOptions struct {
	Topic  string
	Router *routing.ConsumerRouter
}

type ServiceClientsOptions struct {
	Email *email.Client
	Kafka *kafka.Client
}

type ServiceOptions struct {
	Clients       ServiceClientsOptions
	EmailBuild    ServiceEmailBuildOptions
	KafkaDelivery ServiceKafkaDeliveryOptions
	Logger        *zerolog.Logger
}

func NewService(options *ServiceOptions) *Service {
	return &Service{
		clients: clients{
			email: options.Clients.Email,
			kafka: options.Clients.Kafka,
		},
		emailMsgBuild: emailMsgBuildOpts{
			from:         options.EmailBuild.From,
			organization: options.EmailBuild.Organization,
			userAgent:    options.EmailBuild.UserAgent,
			dkimCert:     options.EmailBuild.DKIMCert,
		},
		kafkaDelivery: kafkaDeliveryOpts{
			topic:  options.KafkaDelivery.Topic,
			router: options.KafkaDelivery.Router,
		},
		logger: options.Logger,
	}
}

func (s *Service) Start(_ context.Context) error {
	s.kafkaDelivery.router.OnRecordsFrom(s.kafkaDelivery.topic, func(records []*kgo.Record) {
		for _, record := range records {
			var request emailv1.SendEmailRequest
			if err := proto.Unmarshal(record.Value, &request); err != nil {
				s.logger.Error().Err(err).Msgf(
					"Failed to unmarshal email request from Kafka record received from topic '%s' partition '%d' at offset '%d'",
					record.Topic, record.Partition, record.Offset,
				)
				continue
			}

			message, err := s.buildMessageFromProto(&request)
			if err != nil {
				s.logger.Error().Err(err).Msgf(
					"Failed to build email message from proto for Kafka record received from topic '%s' partition '%d' at offset '%d'",
					record.Topic, record.Partition, record.Offset,
				)
				continue
			}

			err = s.clients.email.Send(email.Request{
				SendOptions: email.SendEmailOptions{
					Email: message,
					SendOptions: &smtp.MailOptions{
						Return:     smtp.DSNReturnHeaders,
						EnvelopeID: request.GetMessageId(),
					},
					ReceiveOptions: &smtp.RcptOptions{
						Notify:                []smtp.DSNNotify{smtp.DSNNotifyFailure},
						OriginalRecipientType: smtp.DSNAddressTypeRFC822,
					},
				},
				Response: make(chan error, 1),
			})
			if err != nil {
				s.logger.Error().Err(err).Msgf(
					"Failed to send email for Kafka record received from topic '%s' partition '%d' at offset '%d'",
					record.Topic, record.Partition, record.Offset,
				)
				continue
			}
		}
	})
	return nil
}

func (s *Service) Stop(_ context.Context) {
	s.logger.Debug().Msg("Shutting down email service")
}

func (s *Service) Send(ctx context.Context, request *emailv1.SendEmailRequest) error {
	if request.GetEmail().GetFrom() == nil {
		request.GetEmail().From = &emailv1.EmailAddress{
			Email: s.emailMsgBuild.from,
		}
	}

	if err := protovalidate.Validate(request); err != nil {
		return fmt.Errorf("email service can't send email because of the validation error: %w", err)
	}

	payload, err := proto.Marshal(request)
	if err != nil {
		return fmt.Errorf("email service can't send email because of the marshaling error: %w", err)
	}

	s.clients.kafka.Driver.Produce(ctx, &kgo.Record{
		Topic: s.kafkaDelivery.topic,
		Key:   []byte(request.GetMessageId()),
		Value: payload,
	}, func(record *kgo.Record, err error) {
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to produce email record to Kafka")
			return
		}
		s.logger.Info().Msgf(
			"Email record produced to Kafka topic %s partition %d at offset %d",
			record.Topic, record.Partition, record.Offset,
		)
	})
	return nil
}

func (s *Service) buildMessageFromProto(request *emailv1.SendEmailRequest) (*mail.Msg, error) {
	emailFromRequest := request.GetEmail()
	message := mail.NewMsg()

	if emailFromRequest.GetFrom().Name == nil {
		if err := message.From(emailFromRequest.GetFrom().GetEmail()); err != nil {
			return nil, fmt.Errorf(
				"failed to set email `from` address to address '%s': %w",
				emailFromRequest.GetFrom().GetEmail(), ErrInvalidEmailRequest,
			)
		}
	} else {
		if err := message.FromFormat(emailFromRequest.GetFrom().GetName(), emailFromRequest.GetFrom().GetEmail()); err != nil {
			return nil, fmt.Errorf(
				"failed to set email `from` address to name '%s' address '%s': %w",
				emailFromRequest.GetFrom().GetName(), emailFromRequest.GetFrom().GetEmail(), ErrInvalidEmailRequest,
			)
		}
	}

	for _, to := range emailFromRequest.GetTo() {
		if to.Name != nil {
			if err := message.AddToFormat(to.GetName(), to.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `to` address to name '%s' address '%s': %w",
					to.GetName(), to.GetEmail(), err,
				)
			}
		} else {
			if err := message.AddTo(to.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `to` address to '%s': %w",
					to.GetEmail(), err,
				)
			}
		}
	}

	for _, cc := range emailFromRequest.GetCc() {
		if cc.Name != nil {
			if err := message.AddCcFormat(cc.GetName(), cc.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `cc` address to name '%s' address '%s': %w",
					cc.GetName(), cc.GetEmail(), err,
				)
			}
		} else {
			if err := message.AddCc(cc.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `cc` address to '%s': %w",
					cc.GetEmail(), err,
				)
			}
		}
	}

	for _, bcc := range emailFromRequest.GetBcc() {
		if bcc.Name != nil {
			if err := message.AddBccFormat(bcc.GetName(), bcc.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `bcc` address to name '%s' address '%s': %w",
					bcc.GetName(), bcc.GetEmail(), err,
				)
			}
		} else {
			if err := message.AddBcc(bcc.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `bcc` address to '%s': %w",
					bcc.GetEmail(), err,
				)
			}
		}
	}

	if emailFromRequest.GetReplyTo() != nil {
		replyTo := emailFromRequest.GetReplyTo()

		if replyTo.Name != nil {
			if err := message.ReplyToFormat(replyTo.GetName(), replyTo.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `reply-to` address to name '%s' address '%s': %w",
					replyTo.GetName(), replyTo.GetEmail(), err,
				)
			}
		} else {
			if err := message.ReplyTo(replyTo.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `reply-to` address to '%s': %w",
					replyTo.GetEmail(), err,
				)
			}
		}
	}

	for _, mdnTo := range emailFromRequest.GetMdnTo() {
		if mdnTo.Name != nil {
			if err := message.RequestMDNAddToFormat(mdnTo.GetName(), mdnTo.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `mdn-to` address to name '%s' address '%s': %w",
					mdnTo.GetName(), mdnTo.GetEmail(), err,
				)
			}
		} else {
			if err := message.RequestMDNAddTo(mdnTo.GetEmail()); err != nil {
				return nil, fmt.Errorf(
					"failed to set email `mdn-to` address to '%s': %w",
					mdnTo.GetEmail(), err,
				)
			}
		}
	}

	message.Subject(emailFromRequest.GetSubject())

	switch emailFromRequest.GetContentMode() {
	case emailv1.ContentMode_CONTENT_MODE_RAW:
		if emailFromRequest.GetRaw().GetText() != "" {
			message.AddAlternativeString(mail.TypeTextPlain, emailFromRequest.GetRaw().GetText())
		}
		if emailFromRequest.GetRaw().GetHtml() != "" {
			message.AddAlternativeString(mail.TypeTextHTML, emailFromRequest.GetRaw().GetHtml())
		}
	case emailv1.ContentMode_CONTENT_MODE_TEMPLATE:
		// @fixme implement template rendering
		return nil, fmt.Errorf(
			"email content mode '%s' is not implemented yet: %w",
			emailFromRequest.GetContentMode().String(), ErrInvalidEmailRequest,
		)
	default:
		return nil, fmt.Errorf(
			"unsupported email content mode '%s': %w",
			emailFromRequest.GetContentMode().String(), ErrInvalidEmailRequest,
		)
	}

	for range emailFromRequest.GetAttachments() {
		// @fixme implement attachment handling
		return nil, fmt.Errorf("email attachments are not implemented yet: %w", ErrInvalidEmailRequest)
	}

	for _, header := range emailFromRequest.GetHeaders() {
		parsedHeader, ok := mapHeader(textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(header.GetName())))
		if !ok {
			return nil, fmt.Errorf("failed to parse unknown header '%s': %w", header.GetName(), ErrInvalidEmailRequest)
		}
		message.SetGenHeader(parsedHeader, header.GetValues()...)
	}

	if emailFromRequest.GetInteractionMode() == emailv1.InteractionMode_INTERACTION_MODE_AUTOMATED {
		message.SetBulk()
	}

	if importance, ok := mapImportance(emailFromRequest.GetImportance()); ok {
		message.SetImportance(importance)
	}

	message.SetOrganization(s.emailMsgBuild.organization)
	message.SetUserAgent(s.emailMsgBuild.userAgent)
	message.SetDateWithValue(request.GetCreatedAt().AsTime())
	message.SetMessageIDWithValue(request.GetMessageId())

	if err := message.SignWithTLSCertificate(s.emailMsgBuild.dkimCert); err != nil {
		return nil, fmt.Errorf("failed to sign email using TLS certificate: %w", err)
	}

	return message, nil
}

func mapHeader(s string) (mail.Header, bool) {
	switch s {
	case "Content-Description":
		return mail.HeaderContentDescription, true
	case "Content-Disposition":
		return mail.HeaderContentDisposition, true
	case "Content-ID":
		return mail.HeaderContentID, true
	case "Content-Language":
		return mail.HeaderContentLang, true
	case "Content-Location":
		return mail.HeaderContentLocation, true
	case "Content-Transfer-Encoding":
		return mail.HeaderContentTransferEnc, true
	case "Content-Type":
		return mail.HeaderContentType, true
	case "Date":
		return mail.HeaderDate, true
	case "Disposition-Notification-To":
		return mail.HeaderDispositionNotificationTo, true
	case "Importance":
		return mail.HeaderImportance, true
	case "In-Reply-To":
		return mail.HeaderInReplyTo, true
	case "List-Unsubscribe":
		return mail.HeaderListUnsubscribe, true
	case "List-Unsubscribe-Post":
		return mail.HeaderListUnsubscribePost, true
	case "Message-ID":
		return mail.HeaderMessageID, true
	case "MIME-Version":
		return mail.HeaderMIMEVersion, true
	case "Organization":
		return mail.HeaderOrganization, true
	case "Precedence":
		return mail.HeaderPrecedence, true
	case "Priority":
		return mail.HeaderPriority, true
	case "References":
		return mail.HeaderReferences, true
	case "Subject":
		return mail.HeaderSubject, true
	case "User-Agent":
		return mail.HeaderUserAgent, true
	case "X-Auto-Response-Suppress":
		return mail.HeaderXAutoResponseSuppress, true
	case "X-Mailer":
		return mail.HeaderXMailer, true
	case "X-MSMail-Priority":
		return mail.HeaderXMSMailPriority, true
	case "X-Priority":
		return mail.HeaderXPriority, true
	default:
		return "", false
	}
}

func mapImportance(level emailv1.ImportanceLevel) (mail.Importance, bool) {
	switch level {
	case emailv1.ImportanceLevel_IMPORTANCE_LEVEL_LOW:
		return mail.ImportanceLow, true

	case emailv1.ImportanceLevel_IMPORTANCE_LEVEL_HIGH:
		return mail.ImportanceHigh, true

	case emailv1.ImportanceLevel_IMPORTANCE_LEVEL_NON_URGENT:
		return mail.ImportanceNonUrgent, true

	case emailv1.ImportanceLevel_IMPORTANCE_LEVEL_URGENT:
		return mail.ImportanceUrgent, true

	default:
		// UNSPECIFIED or unknown values are treated as NORMAL
		return mail.ImportanceNormal, false
	}
}
