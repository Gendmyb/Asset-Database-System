// Package service — webhook dispatcher (event bus subscriber)
package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/webhook"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebhookDispatcher subscribes to event bus and delivers webhooks asynchronously
type WebhookDispatcher struct {
	pool    *pgxpool.Pool
	repo    *repository.WebhookRepo
	engine  *webhook.Engine
}

// NewWebhookDispatcher creates a new dispatcher
func NewWebhookDispatcher(pool *pgxpool.Pool, repo *repository.WebhookRepo) *WebhookDispatcher {
	store := &repoSecretStore{pool: pool, repo: repo}
	return &WebhookDispatcher{
		pool:   pool,
		repo:   repo,
		engine: webhook.NewEngine(store),
	}
}

// Start begins listening for events
func (d *WebhookDispatcher) Start(ctx context.Context) {
	// Subscribe to all events
	ch, err := event.DefaultBus.Subscribe(ctx, "*")
	if err != nil {
		slog.Error("webhook dispatcher: failed to subscribe to event bus", "error", err)
		return
	}

	slog.Info("webhook dispatcher: started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("webhook dispatcher: stopped")
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			go d.handleEvent(ctx, evt)
		}
	}
}

func (d *WebhookDispatcher) handleEvent(ctx context.Context, evt *event.Event) {
	// Find matching active endpoints
	endpoints, err := d.repo.ListActiveByEvent(ctx, d.pool, evt.Type)
	if err != nil {
		slog.Error("webhook dispatcher: failed to list endpoints", "event", evt.Type, "error", err)
		return
	}

	if len(endpoints) == 0 {
		return
	}

	// Marshal event data
	data, _ := json.Marshal(evt)

	for _, ep := range endpoints {
		payload := &webhook.Payload{
			EventID:   evt.ID,
			EventType: evt.Type,
			Data:      data,
		}

		go func(endpoint repository.WebhookEndpointRow) {
			err := d.engine.Deliver(ctx, endpoint.URL, endpoint.ID, payload)
			status := "success"
			var statusCode *int
			var errStr *string
			if err != nil {
				status = "failed"
				code := 0
				statusCode = &code
				e := err.Error()
				errStr = &e
				slog.Warn("webhook dispatcher: delivery failed",
					"endpoint", endpoint.ID, "url", endpoint.URL, "error", err)
			}
			if _, recErr := d.repo.RecordDelivery(ctx, d.pool, endpoint.ID, evt.Type, status, statusCode, errStr); recErr != nil {
				slog.Error("webhook dispatcher: failed to record delivery", "error", recErr)
			}
		}(ep)
	}
}

// repoSecretStore adapts WebhookRepo to webhook.SecretStore interface
type repoSecretStore struct {
	pool *pgxpool.Pool
	repo *repository.WebhookRepo
}

func (s *repoSecretStore) GetSecret(ctx context.Context, endpointID string) ([]byte, error) {
	// We need a lightweight way to get just the secret.
	// Reuse GetEndpoint which fetches the full row.
	row, err := s.repo.GetEndpoint(ctx, s.pool, endpointID, "")
	if err != nil {
		return nil, err
	}
	return row.Secret, nil
}
