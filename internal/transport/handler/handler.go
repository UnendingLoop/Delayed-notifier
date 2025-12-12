// Package handler provides methods to handle api-requests
package handler

import (
	"context"
	"errors"
	"time"

	"github.com/UnendingLoop/delayed-notifier/internal/repository"
	"github.com/UnendingLoop/delayed-notifier/internal/service"
	"github.com/wb-go/wbf/ginext"
)

type Handler struct {
	svc *service.NotificationService
}

func NewHandler(srv *service.NotificationService) *Handler {
	return &Handler{svc: srv}
}

func (H *Handler) SimplePinger(ctx *ginext.Context) {
	ctx.JSON(200, map[string]string{"message": "pong"})
}

func (H *Handler) CreateTask(ctx *ginext.Context) {
	var req struct {
		Text   string `json:"text"`
		SendAt string `json:"send_at"` // формат "2006-01-02 15:04:05"
	}

	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(400, map[string]string{"error": "invalid payload"})
		return
	}

	sendTime, err := time.Parse("2006-01-02 15:04:05", req.SendAt)
	if err != nil {
		ctx.JSON(400, map[string]string{"error": "invalid send_at format"})
		return
	}

	task, err := H.svc.Create(ctx.Request.Context(), req.Text, sendTime)
	if err != nil {
		ctx.JSON(errorChecker(err), map[string]string{"error": err.Error()})
		return
	}

	ctx.JSON(200, map[string]string{"id": task.ID, "status": task.Status})
}

func (H *Handler) GetTask(ctx *ginext.Context) {
	reqID, ok := ctx.Params.Get("uid")
	if !ok {
		ctx.JSON(400, map[string]string{"error": "invalid task uid"})
		return
	}

	task, err := H.svc.GetByID(ctx.Request.Context(), reqID)
	if err != nil {
		ctx.JSON(errorChecker(err), map[string]string{"error": err.Error()})
		return
	}

	ctx.JSON(200, task)
}

func (H *Handler) GetPendingTasks(ctx *ginext.Context) {
	tasks, err := H.svc.GetPending(ctx.Request.Context())
	if err != nil {
		ctx.JSON(errorChecker(err), map[string]string{"error": err.Error()})
		return
	}

	ctx.JSON(200, tasks)
}

func (H *Handler) DeleteTask(ctx *ginext.Context) {
	reqID, ok := ctx.Params.Get("uid")
	if !ok {
		ctx.JSON(400, map[string]string{"error": "task uid not specified"})
		return
	}

	if err := H.svc.Delete(ctx.Request.Context(), reqID); err != nil {
		ctx.JSON(errorChecker(err), map[string]string{"error": err.Error()})
		return
	}
	ctx.JSON(204, nil)
}

func (H *Handler) GetAll(ctx *ginext.Context) {
	tasks, err := H.svc.GetAll(ctx.Request.Context())
	if err != nil {
		ctx.JSON(errorChecker(err), map[string]string{"error": err.Error()})
		return
	}

	ctx.JSON(200, tasks)
}

func errorChecker(err error) int {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return 404
	case errors.Is(err, service.ErrEmptyID) ||
		errors.Is(err, service.ErrEmptyDescription) ||
		errors.Is(err, service.ErrIncorrectTime):
		return 400
	case errors.Is(err, context.DeadlineExceeded):
		return 504
	default:
		return 500
	}
}
