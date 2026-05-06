package service

import (
	"context"
	"errors"
	"fmt"

	"delayednotifier/internal/entity"

	"github.com/wb-go/wbf/logger"
)

func (s *NotifyService) GetTelegramStartHandler() func(ctx context.Context, username string, chatID *int64, startPayload string) error {
	return func(ctx context.Context, username string, chatID *int64, startPayload string) error {
		const op = "service.TelegramStartHandler"
		log := s.log.With("op", op, "username", username, "chat_id", chatID)

		if startPayload != "" {
			err := s.LinkTelegramByToken(ctx, startPayload, chatID)
			if err == nil {
				log.LogAttrs(ctx, logger.InfoLevel, "account linked via token")
				return nil
			}
			log.LogAttrs(ctx, logger.WarnLevel, "failed to link via token, falling back",
				logger.Any("error", err),
			)
		}

		user, err := s.userRepo.GetByTelegramID(ctx, nil, chatID)

		switch {
		case errors.Is(err, entity.ErrDataNotFound):
			regReq := RegisterUserRequest{
				Name:       username,
				TelegramID: chatID,
			}
			_, err = s.RegisterUser(ctx, regReq)
			if err != nil {
				return fmt.Errorf("%s: register new user: %w", op, err)
			}
			log.LogAttrs(ctx, logger.InfoLevel, "new user registered via telegram")

		case err != nil:
			return fmt.Errorf("%s: check existing user: %w", op, err)

		default:
			log.LogAttrs(ctx, logger.InfoLevel, "existing user found",
				logger.Any("user_id", user.ID),
			)
		}

		return nil
	}
}
