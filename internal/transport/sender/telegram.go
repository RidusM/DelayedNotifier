// sender/telegram.go
package sender

import (
	"context"
	"delayednotifier/internal/entity"
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/wb-go/wbf/logger"
)

type TelegramSender struct {
	bot *tgbotapi.BotAPI
	log logger.Logger
}

func NewTelegramSender(botToken string, log logger.Logger) (*TelegramSender, error) {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	log.LogAttrs(context.Background(), logger.InfoLevel, "telegram sender initialized",
		logger.String("bot_username", bot.Self.UserName),
	)

	return &TelegramSender{
		bot: bot,
		log: log,
	}, nil
}

func (s *TelegramSender) Send(ctx context.Context, notification entity.Notification) error {
	chatID, err := strconv.ParseInt(notification.RecipientIdentifier, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat_id '%s': %w", notification.RecipientIdentifier, err)
	}

	msg := tgbotapi.NewMessage(chatID, notification.Payload)
	msg.ParseMode = "HTML" // Поддержка HTML форматирования

	s.log.LogAttrs(ctx, logger.DebugLevel, "sending telegram message",
		logger.Int64("chat_id", chatID),
		logger.String("notification_id", notification.ID.String()),
	)

	_, err = s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	s.log.LogAttrs(ctx, logger.InfoLevel, "telegram message sent",
		logger.Int64("chat_id", chatID),
		logger.String("notification_id", notification.ID.String()),
	)

	return nil
}
