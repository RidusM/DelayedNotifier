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

func (s *TelegramSender) Send(ctx context.Context, n entity.Notification) error {
	const op = "sender.telegram.Send"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: context error: %w", op, err)
	}

	recipient := n.RecipientIdentifier

	if strings.HasPrefix(recipient, "@") {
		return s.sendMessageByUsername(ctx, op, recipient, n)
	} else {
		return s.sendMessageByChatID(ctx, op, recipient, n)
	}
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

func (s *TelegramSender) sendMessageByUsername(ctx context.Context, op, username string, n entity.Notification) error {
	if len(username) <= 1 {
		return fmt.Errorf("%s: invalid username %q: too short", op, username)
	}

	textToSend := s.extractTextFromPayload(n.Payload)

	msg := tgbotapi.NewMessageToChannel(username, textToSend)
	msg.ParseMode = "Markdown"

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending telegram message by username",
		logger.String("username", username),
		logger.String("notification_id", n.ID.String()),
	)

	_, err := s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("%s: send to username %q failed: %w", op, username, err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "telegram message sent successfully by username",
		logger.String("notification_id", n.ID.String()),
		logger.String("username", username),
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
