// nolint:revive,staticcheck
package handler

import (
	"fmt"
	"net/http"
	"time"

	"delayednotifier/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Register a new user
// @Description Registers a user to receive notifications via Email or Telegram
// @Tags Users
// @Accept json
// @Produce json
// @Param request body RegisterUserRequest true "User registration data"
// @Success 201 {object} UserRegisteredResponse "User registered successfully"
// @Failure 400 {object} ErrorResponse "Invalid input data"
// @Failure 409 {object} ErrorResponse "Email already exists"
// @Router /users [post]
func (h *NotifyHandler) RegisterUser(c *gin.Context) {
	const op = "transport.handler.RegisterUser"
	ctx := c.Request.Context()

	var req RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_input", "Validation failed", err)
		return
	}

	serviceReq := service.RegisterUserRequest{
		Name:  req.Name,
		Email: req.Email,
	}

	user, err := h.svc.RegisterUser(ctx, serviceReq)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	response := UserRegisteredResponse{
		UserID:  user.ID,
		Message: msgRegisteredViaEmail,
	}

	h.respondJSON(c, http.StatusCreated, response)
}

// @Summary Generate Telegram Link Token
// @Description Generates a one-time token to link the user's account with Telegram bot
// @Tags Users
// @Accept json
// @Produce json
// @Param user_id path string true "User UUID"
// @Success 200 {object} LinkTokenResponse "Link token and instruction"
// @Failure 404 {object} ErrorResponse "User not found"
// @Router /users/{user_id}/link-token [post]
func (h *NotifyHandler) GenerateLinkToken(c *gin.Context) {
	const op = "transport.handler.GenerateLinkToken"

	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid User ID", err)
		return
	}

	token, err := h.svc.GenerateLinkToken(c.Request.Context(), userID)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	linkURL := fmt.Sprintf("https://t.me/%s?start=%s", h.botCfg.Alias, token)

	response := LinkTokenResponse{
		Token:     token,
		Link:      linkURL,
		Message:   msgLinkTokenGenerated,
		ExpiresIn: linkTokenExpiration,
	}

	h.respondJSON(c, http.StatusOK, response)
}

// @Summary Create a scheduled notification
// @Description Schedules a notification to be sent to a specific user at a given time
// @Tags Notifications
// @Accept json
// @Produce json
// @Param request body CreateNotificationRequest true "Notification details"
// @Success 201 {object} NotificationCreatedResponse "Notification created"
// @Failure 400 {object} ErrorResponse "Invalid input data"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /notify [post]
func (h *NotifyHandler) CreateNotification(c *gin.Context) {
	const op = "transport.handler.CreateNotification"
	ctx := c.Request.Context()

	var req CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_input", "Validation failed", err)
		return
	}

	if req.ScheduledAt.Before(time.Now().UTC()) {
		h.respondError(c, http.StatusBadRequest, "invalid_time", "Scheduled time must be in the future", nil)
		return
	}

	serviceReq := service.CreateNotificationRequest{
		UserID:      req.UserID,
		Channel:     req.Channel,
		Payload:     req.Payload,
		ScheduledAt: req.ScheduledAt,
	}

	id, err := h.svc.CreateNotify(ctx, serviceReq)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.Header("Location", fmt.Sprintf("/notify/%s", id.String()))

	response := NotificationCreatedResponse{
		ID:      id,
		Message: msgNotificationCreated,
	}

	h.respondJSON(c, http.StatusCreated, response)
}

// @Summary Get notification status
// @Description Returns the current status of a notification by its ID
// @Tags Notifications
// @Accept json
// @Produce json
// @Param id path string true "Notification UUID"
// @Success 200 {object} entity.Notification "Notification details"
// @Failure 400 {object} ErrorResponse "Invalid ID format"
// @Failure 404 {object} ErrorResponse "Notification not found"
// @Router /notify/{id} [get]
func (h *NotifyHandler) GetStatus(c *gin.Context) {
	const op = "transport.handler.GetStatus"
	ctx := c.Request.Context()

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid UUID format", err)
		return
	}

	notification, err := h.svc.GetStatus(ctx, id)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	h.respondJSON(c, http.StatusOK, notification)
}

// @Summary Cancel a notification
// @Description Cancels a scheduled notification if it hasn't been sent yet
// @Tags Notifications
// @Accept json
// @Produce json
// @Param id path string true "Notification UUID"
// @Success 200 {object} SuccessResponse "Cancellation successful"
// @Failure 400 {object} ErrorResponse "Invalid ID format"
// @Failure 404 {object} ErrorResponse "Notification not found"
// @Failure 409 {object} ErrorResponse "Notification already sent or cancelled"
// @Router /notify/{id} [delete]
func (h *NotifyHandler) CancelNotification(c *gin.Context) {
	const op = "transport.handler.CancelNotification"
	ctx := c.Request.Context()

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid UUID format", err)
		return
	}

	if err = h.svc.Cancel(ctx, id); err != nil {
		h.handleServiceError(c, err)
		return
	}

	response := SuccessResponse{
		Message: msgNotificationCancelled,
	}

	h.respondJSON(c, http.StatusOK, response)
}

// @Summary Health check endpoint
// @Description Return service status and current timestamp. No authentication required.
// @Tags System
// @Produce json
// @Success 200 {object} map[string]string "Service is healthy"
// @Router /health [get]
func (h *NotifyHandler) Health(c *gin.Context) {
	response := map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	}
	h.respondJSON(c, http.StatusOK, response)
}

func (h *NotifyHandler) respondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

func (h *NotifyHandler) respondError(c *gin.Context, status int, code, message string, err error) {
	response := ErrorResponse{
		Error: message,
		Code:  code,
	}
	if err != nil {
		response.Details = err.Error()
	}
	h.respondJSON(c, status, response)
}
