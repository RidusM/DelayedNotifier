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
	Name  string `json:"name"  binding:"required,min=1,max=100" example:"John Doe"`
	Email string `json:"email" binding:"required,email"         example:"john.doe@example.com"`
}

// swagger:model CreateNotificationRequest
type CreateNotificationRequest struct {
	UserID      uuid.UUID      `json:"user_id"      binding:"required,uuid"                 example:"550e8400-e29b-41d4-a716-446655440001"`
	Channel     entity.Channel `json:"channel"      binding:"required,oneof=telegram email" example:"telegram"`
	Payload     string         `json:"payload"      binding:"required,max=100000"           example:"Don't forget to check the server status!"`
	ScheduledAt time.Time      `json:"scheduled_at" binding:"required"                      example:"2026-05-08T12:00:00Z"`
}

// swagger:model LinkTokenResponse
type LinkTokenResponse struct {
	Token     string `json:"token"      binding:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	Link      string `json:"link"       binding:"required" example:"https://t.me/mybot?start=abc123"`
	Message   string `json:"message"                       example:"Click the link in Telegram to link your account"`
	ExpiresIn string `json:"expires_in" binding:"required" example:"1 hour"`
}

// swagger:model NotificationCreatedResponse
type NotificationCreatedResponse struct {
	ID      uuid.UUID `json:"id"      binding:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440002"`
	Message string    `json:"message"                         example:"Notification scheduled successfully"`
}

// swagger:model UserRegisteredResponse
type UserRegisteredResponse struct {
	// binding:"required,uuid"
	UserID  uuid.UUID `json:"user_id" binding:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440003"`
	Message string    `json:"message"                         example:"Registered via Email"`
}

// swagger:model ErrorResponse
type ErrorResponse struct {
	Error   string `json:"error"             example:"validation failed"`
	Code    string `json:"code,omitempty"    example:"invalid_data"`
	Details string `json:"details,omitempty" example:"Field: 'Email', Error: 'email'"`
}

// swagger:model SuccessResponse
type SuccessResponse struct {
	Message string `json:"message" example:"Operation completed successfully"`
}

// swagger:model HealthResponse
type HealthResponse struct {
	Status string    `json:"status" example:"ok"`
	Time   time.Time `json:"time"   example:"2026-05-08T06:04:15Z"`
}
