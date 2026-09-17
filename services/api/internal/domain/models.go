package domain

import "time"

type FieldType string

const (
	FieldText   FieldType = "text"
	FieldNumber FieldType = "number"
	FieldBool   FieldType = "boolean"
)

type FieldDefinition struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required"`
}

type WorkflowDefinition struct {
	ID        string            `json:"id"`
	TenantID  string            `json:"tenantId"`
	Name      string            `json:"name"`
	Fields    []FieldDefinition `json:"fields"`
	States    []string          `json:"states"`
	Version   int64             `json:"version"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type Record struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenantId"`
	WorkflowID  string         `json:"workflowId"`
	State       string         `json:"state"`
	Data        map[string]any `json:"data"`
	Version     int64          `json:"version"`
	Idempotency string         `json:"idempotencyKey,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type AuditEvent struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenantId"`
	RecordID   string         `json:"recordId"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurredAt"`
	Payload    map[string]any `json:"payload"`
}
