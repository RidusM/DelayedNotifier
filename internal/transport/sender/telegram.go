package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"delayednotifier/internal/entity"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/wb-go/wbf/logger"
)

const (
	_maxIdleConns        = 10
	_pollingTimeout      = 80 * time.Second
	_idleConnTimeout     = 90 * time.Second
	_tlsHandshakeTimeout = 15 * time.Second
)

type TelegramSender struct {
	bot *tgbotapi.BotAPI
	log logger.Logger
}

func NewTelegramSender(botToken string, log logger.Logger) (*TelegramSender, error) {
	client := &http.Client{
		Timeout: _pollingTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        _maxIdleConns,
			IdleConnTimeout:     _idleConnTimeout,
			TLSHandshakeTimeout: _tlsHandshakeTimeout,
		},
	}
	bot, err := tgbotapi.NewBotAPIWithClient(botToken, tgbotapi.APIEndpoint, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	return &TelegramSender{
		bot: bot,
		log: log,
	}, nil
}

func (s *TelegramSender) StartPolling(
	ctx context.Context,
	onSubscribe func(ctx context.Context, username string, chatID *int64, startPayload string) error,
) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if update.Message == nil || !update.Message.IsCommand() {
				continue
			}

			if update.Message.Command() != "start" {
				continue
			}

			username := update.Message.From.UserName
			if username == "" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Для привязки аккаунта необходим username в Telegram.")
				_, _ = s.bot.Send(msg)
				continue
			}

			chatID := update.Message.Chat.ID

			startPayload := update.Message.CommandArguments()

			s.log.LogAttrs(ctx, logger.DebugLevel, "received /start command",
				logger.String("username", username),
				logger.Int64("chat_id", chatID),
				logger.String("payload", startPayload),
			)

			if err := onSubscribe(ctx, username, &chatID, startPayload); err != nil {
				s.log.LogAttrs(ctx, logger.ErrorLevel, "failed to handle subscription",
					logger.String("username", username),
					logger.Any("error", err))

				msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при привязке аккаунта. Попробуйте позже.")
				_, _ = s.bot.Send(msg)
				continue
			}

			var responseText string
			if startPayload != "" {
				responseText = "✅ Аккаунт успешно привязан! Теперь вы будете получать уведомления."
			} else {
				responseText = "✅ Вы успешно зарегистрированы в системе уведомлений."
			}

			msg := tgbotapi.NewMessage(chatID, responseText)
			_, _ = s.bot.Send(msg)

		case <-ctx.Done():
			return
		}
	}
}

func (s *TelegramSender) Send(ctx context.Context, n entity.Notification, recipient string) error {
	const op = "sender.telegram.Send"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: context error: %w", op, err)
	}

	chatID, err := strconv.ParseInt(recipient, 10, 64)
	if err != nil {
		return fmt.Errorf("%s: invalid chat_id %q: %w", op, recipient, err)
	}

	textToSend := s.extractTextFromPayload(n.Payload)

	textToSend = escapeMarkdown(textToSend)

	msg := tgbotapi.NewMessage(chatID, textToSend)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending telegram message",
		logger.Int64("chat_id", chatID),
		logger.String("notification_id", n.ID.String()),
	)

	done := make(chan error, 1)
	go func() {
		_, sendErr := s.bot.Send(msg)
		done <- sendErr
	}()

	select {
	case err = <-done:
		if err != nil {
			return fmt.Errorf("%s: send failed: %w", op, err)
		}
		return nil
	case <-ctx.Done():
		<-done
		return fmt.Errorf("%s: %w", op, ctx.Err())
	case <-time.After(_defaultTimeout):
		<-done
		return fmt.Errorf("%s: timeout after %v", op, _defaultTimeout)
	}
}

func (s *TelegramSender) extractTextFromPayload(payload string) string {
	var p struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err == nil && p.Body != "" {
		return p.Body
	}
	return payload
}

func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}
