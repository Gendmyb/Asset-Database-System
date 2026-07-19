// Package repository — webhook endpoint persistence
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// WebhookEndpointRow represents a webhook endpoint row
type WebhookEndpointRow struct {
	ID        string   `json:"id"`
	OrgID     string   `json:"org_id"`
	URL       string   `json:"url"`
	Secret    []byte   `json:"-"`
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// WebhookDeliveryRow represents a webhook delivery row
type WebhookDeliveryRow struct {
	ID         string `json:"id"`
	EndpointID string `json:"endpoint_id"`
	EventType  string `json:"event_type"`
	Status     string `json:"status"`
	Attempts   int    `json:"attempts"`
	StatusCode *int   `json:"status_code,omitempty"`
	LastError  *string `json:"last_error,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// WebhookRepo webhook endpoint persistence
type WebhookRepo struct{}

// NewWebhookRepo returns a new WebhookRepo
func NewWebhookRepo() *WebhookRepo {
	return &WebhookRepo{}
}

// CreateEndpoint inserts a new webhook endpoint
func (r *WebhookRepo) CreateEndpoint(ctx context.Context, q DBTX, row *WebhookEndpointRow) (string, error) {
	id := uuid.New().String()
	err := q.QueryRow(ctx,
		`INSERT INTO assets.webhook_endpoints (id, org_id, url, secret, events, active)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		id, row.OrgID, row.URL, row.Secret, row.Events, row.Active,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create webhook endpoint: %w", err)
	}
	return id, nil
}

// GetEndpoint retrieves a single webhook endpoint
func (r *WebhookRepo) GetEndpoint(ctx context.Context, q DBTX, id, orgID string) (*WebhookEndpointRow, error) {
	row := &WebhookEndpointRow{}
	var events []string
	err := q.QueryRow(ctx,
		`SELECT id, org_id, url, secret, events, active,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.webhook_endpoints
		 WHERE id=$1 AND org_id=$2`, id, orgID,
	).Scan(&row.ID, &row.OrgID, &row.URL, &row.Secret, &events,
		&row.Active, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get webhook endpoint: %w", err)
	}
	row.Events = events
	return row, nil
}

// ListEndpoints lists all webhook endpoints for an org
func (r *WebhookRepo) ListEndpoints(ctx context.Context, q DBTX, orgID string) ([]WebhookEndpointRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, org_id, url, secret, events, active,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.webhook_endpoints
		 WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list webhook endpoints: %w", err)
	}
	defer rows.Close()

	var results []WebhookEndpointRow
	for rows.Next() {
		var row WebhookEndpointRow
		var events []string
		if err := rows.Scan(&row.ID, &row.OrgID, &row.URL, &row.Secret, &events,
			&row.Active, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		row.Events = events
		results = append(results, row)
	}
	return results, rows.Err()
}

// UpdateEndpoint updates a webhook endpoint
func (r *WebhookRepo) UpdateEndpoint(ctx context.Context, q DBTX, id, orgID string, url *string, events []string, active *bool) error {
	// Dynamic update — simple approach: read-modify-write
	existing, err := r.GetEndpoint(ctx, q, id, orgID)
	if err != nil {
		return err
	}
	if url != nil {
		existing.URL = *url
	}
	if events != nil {
		existing.Events = events
	}
	if active != nil {
		existing.Active = *active
	}
	_, err = q.Exec(ctx,
		`UPDATE assets.webhook_endpoints
		 SET url=$1, events=$2, active=$3, updated_at=now()
		 WHERE id=$4 AND org_id=$5`,
		existing.URL, existing.Events, existing.Active, id, orgID)
	if err != nil {
		return fmt.Errorf("update webhook endpoint: %w", err)
	}
	return nil
}

// DeleteEndpoint deletes a webhook endpoint
func (r *WebhookRepo) DeleteEndpoint(ctx context.Context, q DBTX, id, orgID string) error {
	tag, err := q.Exec(ctx,
		`DELETE FROM assets.webhook_endpoints WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return fmt.Errorf("delete webhook endpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("webhook endpoint not found")
	}
	return nil
}

// ListActiveByEvent returns all active webhook endpoints subscribed to a given event type
func (r *WebhookRepo) ListActiveByEvent(ctx context.Context, q DBTX, eventType string) ([]WebhookEndpointRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, org_id, url, secret, events, active,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.webhook_endpoints
		 WHERE active=true AND ($1 = ANY(events) OR events @> ARRAY['*'])`, eventType)
	if err != nil {
		return nil, fmt.Errorf("list active webhook endpoints: %w", err)
	}
	defer rows.Close()

	var results []WebhookEndpointRow
	for rows.Next() {
		var row WebhookEndpointRow
		var events []string
		if err := rows.Scan(&row.ID, &row.OrgID, &row.URL, &row.Secret, &events,
			&row.Active, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		row.Events = events
		results = append(results, row)
	}
	return results, rows.Err()
}

// RecordDelivery inserts a webhook delivery log
func (r *WebhookRepo) RecordDelivery(ctx context.Context, q DBTX, endpointID, eventType, status string, statusCode *int, lastError *string) (string, error) {
	id := uuid.New().String()
	_, err := q.Exec(ctx,
		`INSERT INTO assets.webhook_deliveries (id, endpoint_id, event_type, status, status_code, last_error)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		id, endpointID, eventType, status, statusCode, lastError)
	if err != nil {
		return "", fmt.Errorf("record webhook delivery: %w", err)
	}
	return id, nil
}

// ListDeliveries lists deliveries for an endpoint
func (r *WebhookRepo) ListDeliveries(ctx context.Context, q DBTX, endpointID string, limit int) ([]WebhookDeliveryRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := q.Query(ctx,
		`SELECT id, endpoint_id, event_type, status, attempts, status_code, last_error,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.webhook_deliveries
		 WHERE endpoint_id=$1 ORDER BY created_at DESC LIMIT $2`, endpointID, limit)
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	var results []WebhookDeliveryRow
	for rows.Next() {
		var row WebhookDeliveryRow
		if err := rows.Scan(&row.ID, &row.EndpointID, &row.EventType, &row.Status,
			&row.Attempts, &row.StatusCode, &row.LastError, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
