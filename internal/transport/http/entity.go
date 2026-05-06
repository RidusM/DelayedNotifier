// swagger:meta
// nolint:revive,staticcheck
package handler

import (
	"time"

	"delayednotifier/internal/entity"

	"github.com/google/uuid"
)

const (
	msgRegisteredViaEmail    = "Registered via Email"
	msgLinkTokenGenerated    = "Click the link in Telegram to link your account"
	msgNotificationCreated   = "Notification scheduled successfully"
	msgNotificationCancelled = "Notification cancelled"
	linkTokenExpiration      = "1 hour"
)

// swagger:model RegisterUserRequest
type RegisterUserRequest struct {
	Name  string `json:"name"  binding:"required,min=1,max=100"`
	Email string `json:"email" binding:"required,email"`
}

// swagger:model CreateNotificationRequest
type CreateNotificationRequest struct {
	UserID      uuid.UUID      `json:"user_id"      binding:"required,uuid"`
	Channel     entity.Channel `json:"channel"      binding:"required,oneof=telegram email"`
	Payload     string         `json:"payload"      binding:"required,max=100000"`
	ScheduledAt time.Time      `json:"scheduled_at" binding:"required"`
}

// swagger:model LinkTokenResponse
type LinkTokenResponse struct {
	Token     string `json:"token"      binding:"required"`
	Link      string `json:"link"       binding:"required"`
	Message   string `json:"message"`
	ExpiresIn string `json:"expires_in" binding:"required"`
}

// swagger:model NotificationCreatedResponse
type NotificationCreatedResponse struct {
	ID      uuid.UUID `json:"id"      binding:"required,uuid"`
	Message string    `json:"message"`
}

// swagger:model UserRegisteredResponse
type UserRegisteredResponse struct {
	UserID  uuid.UUID `json:"user_id" binding:"required,uuid"`
	Message string    `json:"message"`
}

// swagger:model ErrorResponse
type ErrorResponse struct {
	Error   string `json:"error"             example:"notify not found"`
	Code    string `json:"code,omitempty"    example:"not_found"`
	Details string `json:"details,omitempty" example:"notify with id 123 does not exist"`
}

// swagger:model SuccessResponse
type SuccessResponse struct {
	Message string `json:"message" example:"Operation completed successfully"`
}
