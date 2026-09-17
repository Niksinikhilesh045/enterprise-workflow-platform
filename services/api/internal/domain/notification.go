package domain

import "time"

// Notification is an idempotent projection of a workflow event. The source
// event ID is reused as the MongoDB document ID so Kafka redelivery cannot
// create duplicate notifications.
type Notification struct {
	ID        string    `json:"id" bson:"_id"`
	TenantID  string    `json:"tenantId" bson:"tenantId"`
	RecordID  string    `json:"recordId" bson:"recordId"`
	EventType string    `json:"eventType" bson:"eventType"`
	State     string    `json:"state" bson:"state"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
}
