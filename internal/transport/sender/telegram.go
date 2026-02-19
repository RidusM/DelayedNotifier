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

func (s *TelegramSender) Send(ctx context.Context, n entity.Notification) error {
	const op = "sender.telegram.Send"

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: context error: %w", op, err)
	}

	chatID, err := strconv.ParseInt(n.RecipientIdentifier, 10, 64)
	if err != nil {
		return fmt.Errorf("%s: invalid chat_id %q: %w", op, n.RecipientIdentifier, err)
	}

	textToSend := n.Payload
	var payload struct {
		Body string `json:"body"`
	}
	if unmarshErr := json.Unmarshal([]byte(n.Payload), &payload); unmarshErr == nil && payload.Body != "" {
		textToSend = payload.Body
	}

	msg := tgbotapi.NewMessage(chatID, textToSend)
	msg.ParseMode = "Markdown"

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending telegram message",
		logger.Int64("chat_id", chatID),
		logger.String("notification_id", n.ID.String()),
	)

	_, err = s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("%s: send failed: %w", op, err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "telegram message sent successfully",
		logger.String("notification_id", n.ID.String()),
		logger.Int64("chat_id", chatID),
	)

	return nil
}
