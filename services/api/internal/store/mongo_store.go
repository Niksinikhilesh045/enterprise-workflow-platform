package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	client                                            *mongo.Client
	db                                                *mongo.Database
	workflows, records, idempotency, audit, outbox *mongo.Collection
}

type idempotencyDoc struct {
	TenantID  string    `bson:"tenantId"`
	Key       string    `bson:"key"`
	RecordID  string    `bson:"recordId"`
	CreatedAt time.Time `bson:"createdAt"`
}

func NewMongoStore(ctx context.Context, uri, database string) (*MongoStore, error) {
	if database == "" {
		database = "enterprise_workflow"
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping mongo: %w", err)
	}
	db := client.Database(database)
	s := &MongoStore{client: client, db: db, workflows: db.Collection("workflows"), records: db.Collection("records"), idempotency: db.Collection("idempotency_keys"), audit: db.Collection("audit_events"), outbox: db.Collection("outbox_events")}
	if err := s.ensureIndexes(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *MongoStore) Close(ctx context.Context) error { return s.client.Disconnect(ctx) }

func (s *MongoStore) ensureIndexes(ctx context.Context) error {
	_, err := s.idempotency.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)})
	return err
}

func (s *MongoStore) CreateWorkflow(tenantID string, wf domain.WorkflowDefinition) (domain.WorkflowDefinition, error) {
	if tenantID == "" || wf.Name == "" || len(wf.States) == 0 {
		return domain.WorkflowDefinition{}, ErrInvalidWorkflow
	}
	now := time.Now().UTC()
	wf.ID = primitive.NewObjectID().Hex()
	wf.TenantID = tenantID
	wf.Version = 1
	wf.CreatedAt = now
	wf.UpdatedAt = now
	_, err := s.workflows.InsertOne(context.Background(), wf)
	return wf, err
}

func (s *MongoStore) ListWorkflows(tenantID string) ([]domain.WorkflowDefinition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cur, err := s.workflows.Find(ctx, bson.M{"tenantId": tenantID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.WorkflowDefinition
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.WorkflowDefinition{}
	}
	return out, nil
}

func (s *MongoStore) CreateRecord(tenantID, key string, r domain.Record) (domain.Record, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if key != "" {
		if rec, ok, err := s.findReplay(ctx, tenantID, key); err != nil {
			return domain.Record{}, false, err
		} else if ok {
			return rec, true, nil
		}
	}
	var created domain.Record
	sess, err := s.client.StartSession()
	if err != nil {
		return domain.Record{}, false, err
	}
	defer sess.EndSession(ctx)
	_, err = sess.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var wf domain.WorkflowDefinition
		if err := s.workflows.FindOne(sc, bson.M{"_id": r.WorkflowID, "tenantId": tenantID}).Decode(&wf); err != nil {
			return nil, ErrInvalidWorkflow
		}
		now := time.Now().UTC()
		r.ID = primitive.NewObjectID().Hex()
		r.TenantID = tenantID
		r.State = wf.States[0]
		r.Version = 1
		r.Idempotency = key
		r.CreatedAt = now
		r.UpdatedAt = now
		if _, err := s.records.InsertOne(sc, r); err != nil {
			return nil, err
		}
		if key != "" {
			if _, err := s.idempotency.InsertOne(sc, idempotencyDoc{TenantID: tenantID, Key: key, RecordID: r.ID, CreatedAt: now}); err != nil {
				return nil, err
			}
		}
		if err := s.writeAuditAndOutbox(sc, r, "record.created"); err != nil {
			return nil, err
		}
		created = r
		return nil, nil
	})
	if err != nil {
		if key != "" && mongo.IsDuplicateKeyError(err) {
			if rec, ok, replayErr := s.findReplay(ctx, tenantID, key); replayErr == nil && ok {
				return rec, true, nil
			}
		}
		return domain.Record{}, false, err
	}
	return created, false, nil
}

func (s *MongoStore) ListRecords(tenantID, state string) ([]domain.Record, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"tenantId": tenantID}
	if state != "" {
		filter["state"] = state
	}
	cur, err := s.records.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.Record
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.Record{}
	}
	return out, nil
}

func (s *MongoStore) GetRecord(tenantID, id string) (domain.Record, error) {
	var r domain.Record
	err := s.records.FindOne(context.Background(), bson.M{"_id": id, "tenantId": tenantID}).Decode(&r)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Record{}, ErrNotFound
	}
	return r, err
}

func (s *MongoStore) UpdateRecord(tenantID, id string, expectedVersion int64, state string, data map[string]any) (domain.Record, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var updated domain.Record
	sess, err := s.client.StartSession()
	if err != nil {
		return domain.Record{}, err
	}
	defer sess.EndSession(ctx)
	_, err = sess.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		var current domain.Record
		if err := s.records.FindOne(sc, bson.M{"_id": id, "tenantId": tenantID}).Decode(&current); errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, err
		}
		if current.Version != expectedVersion {
			return nil, ErrConflict
		}
		var wf domain.WorkflowDefinition
		if err := s.workflows.FindOne(sc, bson.M{"_id": current.WorkflowID, "tenantId": tenantID}).Decode(&wf); err != nil {
			return nil, ErrInvalidWorkflow
		}
		if state != "" && !contains(wf.States, state) {
			return nil, ErrInvalidWorkflow
		}
		set := bson.M{"updatedAt": time.Now().UTC()}
		if state != "" {
			set["state"] = state
		}
		if data != nil {
			set["data"] = data
		}
		err := s.records.FindOneAndUpdate(sc, bson.M{"_id": id, "tenantId": tenantID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrConflict
		}
		if err != nil {
			return nil, err
		}
		return nil, s.writeAuditAndOutbox(sc, updated, "record.updated")
	})
	return updated, err
}

func (s *MongoStore) ListAuditEvents(tenantID string) ([]domain.AuditEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cur, err := s.audit.Find(ctx, bson.M{"tenantId": tenantID}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.AuditEvent
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.AuditEvent{}
	}
	return out, nil
}

func (s *MongoStore) ListPendingOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	cur, err := s.outbox.Find(ctx, bson.M{"publishedAt": bson.M{"$exists": false}}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.OutboxEvent
	return out, cur.All(ctx, &out)
}

func (s *MongoStore) MarkOutboxPublished(ctx context.Context, id string) error {
	_, err := s.outbox.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"publishedAt": time.Now().UTC(), "lastError": ""}})
	return err
}

func (s *MongoStore) MarkOutboxFailed(ctx context.Context, id, reason string) error {
	_, err := s.outbox.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$inc": bson.M{"attempts": 1}, "$set": bson.M{"lastError": reason}})
	return err
}

func (s *MongoStore) findReplay(ctx context.Context, tenantID, key string) (domain.Record, bool, error) {
	var d idempotencyDoc
	if err := s.idempotency.FindOne(ctx, bson.M{"tenantId": tenantID, "key": key}).Decode(&d); errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Record{}, false, nil
	} else if err != nil {
		return domain.Record{}, false, err
	}
	var r domain.Record
	if err := s.records.FindOne(ctx, bson.M{"_id": d.RecordID, "tenantId": tenantID}).Decode(&r); err != nil {
		return domain.Record{}, false, err
	}
	return r, true, nil
}

func (s *MongoStore) writeAuditAndOutbox(ctx mongo.SessionContext, r domain.Record, eventType string) error {
	now := time.Now().UTC()
	a := domain.AuditEvent{ID: primitive.NewObjectID().Hex(), TenantID: r.TenantID, RecordID: r.ID, Type: eventType, OccurredAt: now, Payload: map[string]any{"state": r.State, "version": r.Version}}
	o := domain.OutboxEvent{ID: primitive.NewObjectID().Hex(), TenantID: r.TenantID, AggregateID: r.ID, Topic: RecordEventsTopic, Key: r.ID, Type: eventType, Payload: map[string]any{"recordId": r.ID, "workflowId": r.WorkflowID, "state": r.State, "version": r.Version}, CreatedAt: now}
	if _, err := s.audit.InsertOne(ctx, a); err != nil {
		return err
	}
	_, err := s.outbox.InsertOne(ctx, o)
	return err
}
