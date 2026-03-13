package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"delayednotifier/internal/entity"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/wb-go/wbf/logger"
)

const (
	_maxIdleConns        = 10
	_idleConnTimeout     = 30 * time.Second
	_tlsHandshakeTimeout = 5 * time.Second
)

type TelegramSender struct {
	bot *tgbotapi.BotAPI
	log logger.Logger
}

func NewTelegramSender(botToken string, log logger.Logger) (*TelegramSender, error) {
	client := &http.Client{
		Timeout: _defaultTimeout,
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
	onSubscribe func(ctx context.Context, username string, chatID int64) error,
) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := s.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if update.Message == nil || update.Message.Text != "/start" {
				continue
			}
			if update.Message.From.UserName == "" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID,
					"Для подписки необходим username в Telegram.")
				if _, botErr := s.bot.Send(msg); botErr != nil {
					s.log.LogAttrs(ctx, logger.ErrorLevel, "To subscribe need yours telegram username",
						logger.Any("error", botErr))
				}
				continue
			}

			username := "@" + update.Message.From.UserName
			chatID := update.Message.Chat.ID

			if err := onSubscribe(ctx, username, chatID); err != nil {
				s.log.LogAttrs(ctx, logger.ErrorLevel, "failed to save subscriber",
					logger.String("username", username), logger.Any("error", err))
				continue
			}

			msg := tgbotapi.NewMessage(chatID, "Вы подписались на уведомления!")
			if _, botErr := s.bot.Send(msg); botErr != nil {
				s.log.LogAttrs(ctx, logger.ErrorLevel, "failed to send message",
					logger.Any("error", botErr))
			}

		case <-ctx.Done():
			return
		}
	}
}

func (s *TelegramSender) Send(ctx context.Context, n entity.Notification) error {
	const op = "sender.telegram.Send"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: context error: %w", op, err)
	}

	return s.sendMessageByChatID(ctx, op, n.RecipientIdentifier, n)
}

func (s *TelegramSender) sendMessageByChatID(ctx context.Context, op, chatIDStr string, n entity.Notification) error {
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%s: invalid chat_id %q: %w", op, chatIDStr, err)
	}

	textToSend := s.extractTextFromPayload(n.Payload)
	msg := tgbotapi.NewMessage(chatID, textToSend)
	msg.ParseMode = "Markdown"

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending telegram message by chat_id",
		logger.Int64("chat_id", chatID),
		logger.String("notification_id", n.ID.String()),
	)

	_, err = s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("%s: send failed: %w", op, err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "telegram message sent successfully by chat_id",
		logger.String("notification_id", n.ID.String()),
		logger.Int64("chat_id", chatID),
	)

	return nil
}

func (s *TelegramSender) extractTextFromPayload(payload string) string {
	textToSend := payload
	var payloadStruct struct {
		Body string `json:"body"`
	}
	if unmarshErr := json.Unmarshal([]byte(payload), &payloadStruct); unmarshErr == nil && payloadStruct.Body != "" {
		textToSend = payloadStruct.Body
	}
	return textToSend
}
