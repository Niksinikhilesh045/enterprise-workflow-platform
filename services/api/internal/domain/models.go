package domain

import "time"

type FieldType string

const (
	FieldText FieldType = "text"
	FieldNumber FieldType = "number"
	FieldBool FieldType = "boolean"
)

type FieldDefinition struct {
	Key string `json:"key" bson:"key"`
	Label string `json:"label" bson:"label"`
	Type FieldType `json:"type" bson:"type"`
	Required bool `json:"required" bson:"required"`
}

type WorkflowDefinition struct {
	ID string `json:"id" bson:"_id"`
	TenantID string `json:"tenantId" bson:"tenantId"`
	Name string `json:"name" bson:"name"`
	Fields []FieldDefinition `json:"fields" bson:"fields"`
	States []string `json:"states" bson:"states"`
	Version int64 `json:"version" bson:"version"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type Record struct {
	ID string `json:"id" bson:"_id"`
	TenantID string `json:"tenantId" bson:"tenantId"`
	WorkflowID string `json:"workflowId" bson:"workflowId"`
	State string `json:"state" bson:"state"`
	Data map[string]any `json:"data" bson:"data"`
	Version int64 `json:"version" bson:"version"`
	Idempotency string `json:"idempotencyKey,omitempty" bson:"idempotencyKey,omitempty"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type AuditEvent struct {
	ID string `json:"id" bson:"_id"`
	TenantID string `json:"tenantId" bson:"tenantId"`
	RecordID string `json:"recordId" bson:"recordId"`
	Type string `json:"type" bson:"type"`
	OccurredAt time.Time `json:"occurredAt" bson:"occurredAt"`
	Payload map[string]any `json:"payload" bson:"payload"`
}

type OutboxEvent struct {
	ID string `json:"id" bson:"_id"`
	TenantID string `json:"tenantId" bson:"tenantId"`
	AggregateID string `json:"aggregateId" bson:"aggregateId"`
	Topic string `json:"topic" bson:"topic"`
	Key string `json:"key" bson:"key"`
	Type string `json:"type" bson:"type"`
	Payload map[string]any `json:"payload" bson:"payload"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
	Attempts int `json:"attempts" bson:"attempts"`
	LastError string `json:"lastError,omitempty" bson:"lastError,omitempty"`
}
