package store

import (
	"context"
	"errors"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (s *MongoStore) StoreNotification(ctx context.Context, notification domain.Notification) (bool, error) {
	_, err := s.db.Collection("notifications").InsertOne(ctx, notification)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *MongoStore) CountNotifications(ctx context.Context, tenantID string) (int64, error) {
	if tenantID == "" {
		return 0, errors.New("tenant ID is required")
	}
	return s.db.Collection("notifications").CountDocuments(ctx, bson.M{"tenantId": tenantID})
}
