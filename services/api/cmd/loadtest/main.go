package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type workflowResponse struct {
	ID string `json:"id"`
}

type requestResult struct {
	Duration time.Duration
	Status   int
	Err      error
}

type pipelineSnapshot struct {
	Records       int64
	Unpublished   int64
	Notifications int64
	KafkaLag      int64
}

type pipelineMilestones struct {
	HTTPComplete          time.Duration
	OutboxPublished       time.Duration
	KafkaCaughtUp         time.Duration
	NotificationsComplete time.Duration
	EndToEnd              time.Duration
}

type pipelineProbe struct {
	mongoClient   *mongo.Client
	records       *mongo.Collection
	outbox        *mongo.Collection
	notifications *mongo.Collection
	kafkaClient   *kafka.Client
	topic         string
	groupID       string
	partitions    []int
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "API base URL")
	token := flag.String("token", "", "bearer token with permission to create workflows and records")
	workflowID := flag.String("workflow-id", "", "existing workflow ID; when empty, the tool creates one")
	requests := flag.Int("requests", 1000, "number of record-create requests")
	concurrency := flag.Int("concurrency", 20, "number of concurrent workers")
	timeout := flag.Duration("timeout", 15*time.Second, "per-request timeout")

	waitForPipeline := flag.Bool("wait-for-pipeline", false, "measure end-to-end completion through outbox, Kafka, and notification projection")
	pipelineTimeout := flag.Duration("pipeline-timeout", 45*time.Second, "maximum time to wait for the asynchronous pipeline")
	pipelinePoll := flag.Duration("pipeline-poll", 100*time.Millisecond, "poll interval while waiting for the asynchronous pipeline")
	mongoURI := flag.String("mongo-uri", envOr("MONGO_URI", "mongodb://localhost:27017/?replicaSet=rs0"), "MongoDB URI used by pipeline benchmark mode")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "enterprise_workflow"), "MongoDB database used by pipeline benchmark mode")
	kafkaBrokers := flag.String("kafka-brokers", envOr("KAFKA_BROKERS", "localhost:9092"), "comma-separated Kafka brokers used by pipeline benchmark mode")
	kafkaTopic := flag.String("kafka-topic", "workflow.record-events", "Kafka topic used by pipeline benchmark mode")
	consumerGroup := flag.String("consumer-group", "workflow-notifications-v1", "Kafka consumer group used by pipeline benchmark mode")
	flag.Parse()

	if strings.TrimSpace(*token) == "" {
		log.Fatal("-token is required")
	}
	if *requests <= 0 {
		log.Fatal("-requests must be greater than zero")
	}
	if *concurrency <= 0 {
		log.Fatal("-concurrency must be greater than zero")
	}
	if *pipelineTimeout <= 0 {
		log.Fatal("-pipeline-timeout must be greater than zero")
	}
	if *pipelinePoll <= 0 {
		log.Fatal("-pipeline-poll must be greater than zero")
	}

	client := &http.Client{Timeout: *timeout}
	base := strings.TrimRight(*baseURL, "/")
	wfID := strings.TrimSpace(*workflowID)
	if wfID == "" {
		var err error
		wfID, err = createWorkflow(client, base, *token)
		if err != nil {
			log.Fatalf("create workflow: %v", err)
		}
		fmt.Printf("created load-test workflow: %s\n", wfID)
	}

	var probe *pipelineProbe
	var baseline pipelineSnapshot
	if *waitForPipeline {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var err error
		probe, err = newPipelineProbe(ctx, *mongoURI, *mongoDB, *kafkaBrokers, *kafkaTopic, *consumerGroup)
		if err == nil {
			baseline, err = probe.Snapshot(ctx, wfID)
		}
		cancel()
		if err != nil {
			if probe != nil {
				_ = probe.Close(context.Background())
			}
			log.Fatalf("initialize pipeline benchmark: %v", err)
		}
		defer probe.Close(context.Background())
	}

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	jobs := make(chan int)
	results := make(chan requestResult, *requests)
	var wg sync.WaitGroup

	workerCount := *concurrency
	if workerCount > *requests {
		workerCount = *requests
	}
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results <- createRecord(client, base, *token, wfID, runID, index)
			}
		}()
	}

	started := time.Now()
	go func() {
		for i := 0; i < *requests; i++ {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	latencies := make([]time.Duration, 0, *requests)
	statusCounts := make(map[int]int)
	successful := 0
	failed := 0
	for result := range results {
		latencies = append(latencies, result.Duration)
		if result.Err != nil {
			failed++
			continue
		}
		statusCounts[result.Status]++
		if result.Status == http.StatusCreated {
			successful++
		} else {
			failed++
		}
	}
	apiElapsed := time.Since(started)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	throughput := float64(*requests) / apiElapsed.Seconds()

	fmt.Println("\nLoad test results")
	fmt.Printf("workflow id:   %s\n", wfID)
	fmt.Printf("requests:      %d\n", *requests)
	fmt.Printf("concurrency:   %d\n", workerCount)
	fmt.Printf("successful:    %d\n", successful)
	fmt.Printf("failed:        %d\n", failed)
	fmt.Printf("elapsed:       %s\n", apiElapsed.Round(time.Millisecond))
	fmt.Printf("throughput:    %.2f req/s\n", throughput)
	fmt.Printf("latency p50:   %s\n", percentile(latencies, 0.50).Round(time.Microsecond))
	fmt.Printf("latency p95:   %s\n", percentile(latencies, 0.95).Round(time.Microsecond))
	fmt.Printf("latency p99:   %s\n", percentile(latencies, 0.99).Round(time.Microsecond))
	fmt.Printf("latency max:   %s\n", maxDuration(latencies).Round(time.Microsecond))
	fmt.Printf("status codes:  %v\n", statusCounts)

	if failed > 0 {
		os.Exit(1)
	}

	if *waitForPipeline {
		ctx, cancel := context.WithTimeout(context.Background(), *pipelineTimeout)
		finalSnapshot, milestones, err := waitUntilPipelineComplete(
			ctx,
			probe,
			wfID,
			baseline,
			int64(successful),
			started,
			apiElapsed,
			*pipelinePoll,
		)
		cancel()

		postHTTPDrain := milestones.EndToEnd - milestones.HTTPComplete
		if postHTTPDrain < 0 {
			postHTTPDrain = 0
		}

		fmt.Println("\nEnd-to-end pipeline results")
		fmt.Printf("records:        %d/%d\n", finalSnapshot.Records-baseline.Records, successful)
		fmt.Printf("unpublished:    %d\n", finalSnapshot.Unpublished)
		fmt.Printf("notifications:  %d/%d\n", finalSnapshot.Notifications-baseline.Notifications, successful)
		fmt.Printf("kafka lag:      %d\n", finalSnapshot.KafkaLag)
		fmt.Printf("post-http drain:%s\n", formatAlignedDuration(postHTTPDrain))
		fmt.Printf("end-to-end:     %s\n", milestones.EndToEnd.Round(time.Millisecond))
		if milestones.EndToEnd > 0 {
			fmt.Printf("e2e throughput: %.2f records/s\n", float64(successful)/milestones.EndToEnd.Seconds())
		}

		fmt.Println("\nPipeline stage timings")
		fmt.Printf("http complete:          %s\n", milestones.HTTPComplete.Round(time.Millisecond))
		fmt.Printf("outbox fully published: %s (%s after HTTP)\n",
			milestones.OutboxPublished.Round(time.Millisecond),
			stageDelta(milestones.OutboxPublished, milestones.HTTPComplete),
		)
		fmt.Printf("kafka lag reached zero: %s (%s after outbox)\n",
			milestones.KafkaCaughtUp.Round(time.Millisecond),
			stageDelta(milestones.KafkaCaughtUp, milestones.OutboxPublished),
		)
		fmt.Printf("notifications complete: %s (%s after HTTP)\n",
			milestones.NotificationsComplete.Round(time.Millisecond),
			stageDelta(milestones.NotificationsComplete, milestones.HTTPComplete),
		)
		fmt.Printf("pipeline complete:      %s\n", milestones.EndToEnd.Round(time.Millisecond))

		if err != nil {
			log.Printf("pipeline benchmark incomplete: %v", err)
			os.Exit(1)
		}
	}
}

func newPipelineProbe(ctx context.Context, mongoURI, database, brokersValue, topic, groupID string) (*pipelineProbe, error) {
	if strings.TrimSpace(mongoURI) == "" {
		return nil, errors.New("MongoDB URI is required")
	}
	if strings.TrimSpace(database) == "" {
		return nil, errors.New("MongoDB database is required")
	}
	if strings.TrimSpace(topic) == "" {
		return nil, errors.New("Kafka topic is required")
	}
	if strings.TrimSpace(groupID) == "" {
		return nil, errors.New("Kafka consumer group is required")
	}

	brokers := splitCSV(brokersValue)
	if len(brokers) == 0 {
		return nil, errors.New("at least one Kafka broker is required")
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("connect MongoDB: %w", err)
	}
	if err := mongoClient.Ping(ctx, nil); err != nil {
		_ = mongoClient.Disconnect(context.Background())
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}

	partitions, err := kafka.LookupPartitions(ctx, "tcp", brokers[0], topic)
	if err != nil {
		_ = mongoClient.Disconnect(context.Background())
		return nil, fmt.Errorf("lookup Kafka partitions: %w", err)
	}
	partitionIDs := make([]int, 0, len(partitions))
	for _, partition := range partitions {
		if partition.Error != nil {
			_ = mongoClient.Disconnect(context.Background())
			return nil, fmt.Errorf("Kafka partition %d metadata: %w", partition.ID, partition.Error)
		}
		partitionIDs = append(partitionIDs, partition.ID)
	}
	sort.Ints(partitionIDs)
	if len(partitionIDs) == 0 {
		_ = mongoClient.Disconnect(context.Background())
		return nil, fmt.Errorf("Kafka topic %q has no partitions", topic)
	}

	db := mongoClient.Database(database)
	return &pipelineProbe{
		mongoClient:   mongoClient,
		records:       db.Collection("records"),
		outbox:        db.Collection("outbox_events"),
		notifications: db.Collection("notifications"),
		kafkaClient: &kafka.Client{
			Addr:    kafka.TCP(brokers...),
			Timeout: 5 * time.Second,
		},
		topic:      topic,
		groupID:    groupID,
		partitions: partitionIDs,
	}, nil
}

func (p *pipelineProbe) Close(ctx context.Context) error {
	if p == nil || p.mongoClient == nil {
		return nil
	}
	return p.mongoClient.Disconnect(ctx)
}

func (p *pipelineProbe) Snapshot(ctx context.Context, workflowID string) (pipelineSnapshot, error) {
	records, err := p.records.CountDocuments(ctx, bson.M{"workflowId": workflowID})
	if err != nil {
		return pipelineSnapshot{}, fmt.Errorf("count records: %w", err)
	}

	unpublished, err := p.outbox.CountDocuments(ctx, bson.M{
		"payload.workflowId": workflowID,
		"type":               "record.created",
		"publishedAt":        bson.M{"$exists": false},
	})
	if err != nil {
		return pipelineSnapshot{}, fmt.Errorf("count unpublished outbox events: %w", err)
	}

	notifications, err := p.countNotificationsForWorkflow(ctx, workflowID)
	if err != nil {
		return pipelineSnapshot{}, err
	}

	lag, err := p.kafkaLag(ctx)
	if err != nil {
		return pipelineSnapshot{}, err
	}

	return pipelineSnapshot{
		Records:       records,
		Unpublished:   unpublished,
		Notifications: notifications,
		KafkaLag:      lag,
	}, nil
}

func (p *pipelineProbe) countNotificationsForWorkflow(ctx context.Context, workflowID string) (int64, error) {
	cursor, err := p.outbox.Find(
		ctx,
		bson.M{
			"payload.workflowId": workflowID,
			"type":               "record.created",
		},
		options.Find().SetProjection(bson.M{"_id": 1}),
	)
	if err != nil {
		return 0, fmt.Errorf("list workflow outbox ids: %w", err)
	}
	defer cursor.Close(ctx)

	var outboxRows []struct {
		ID string `bson:"_id"`
	}
	if err := cursor.All(ctx, &outboxRows); err != nil {
		return 0, fmt.Errorf("decode workflow outbox ids: %w", err)
	}
	if len(outboxRows) == 0 {
		return 0, nil
	}

	ids := make([]string, 0, len(outboxRows))
	for _, row := range outboxRows {
		ids = append(ids, row.ID)
	}

	count, err := p.notifications.CountDocuments(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return 0, fmt.Errorf("count workflow notifications: %w", err)
	}
	return count, nil
}

func (p *pipelineProbe) kafkaLag(ctx context.Context) (int64, error) {
	offsetRequests := make([]kafka.OffsetRequest, 0, len(p.partitions))
	for _, partition := range p.partitions {
		offsetRequests = append(offsetRequests, kafka.LastOffsetOf(partition))
	}
	endOffsetsResponse, err := p.kafkaClient.ListOffsets(ctx, &kafka.ListOffsetsRequest{
		Topics: map[string][]kafka.OffsetRequest{p.topic: offsetRequests},
	})
	if err != nil {
		return 0, fmt.Errorf("list Kafka end offsets: %w", err)
	}

	endOffsets := make(map[int]int64, len(p.partitions))
	for _, partition := range endOffsetsResponse.Topics[p.topic] {
		if partition.Error != nil {
			return 0, fmt.Errorf("Kafka end offset partition %d: %w", partition.Partition, partition.Error)
		}
		endOffsets[partition.Partition] = partition.LastOffset
	}

	committedResponse, err := p.kafkaClient.OffsetFetch(ctx, &kafka.OffsetFetchRequest{
		GroupID: p.groupID,
		Topics:  map[string][]int{p.topic: p.partitions},
	})
	if err != nil {
		return 0, fmt.Errorf("fetch Kafka committed offsets: %w", err)
	}
	if committedResponse.Error != nil {
		return 0, fmt.Errorf("fetch Kafka committed offsets: %w", committedResponse.Error)
	}

	committedOffsets := make(map[int]int64, len(p.partitions))
	for _, partition := range committedResponse.Topics[p.topic] {
		if partition.Error != nil {
			return 0, fmt.Errorf("Kafka committed offset partition %d: %w", partition.Partition, partition.Error)
		}
		committedOffsets[partition.Partition] = partition.CommittedOffset
	}

	for _, partition := range p.partitions {
		if _, ok := endOffsets[partition]; !ok {
			return 0, fmt.Errorf("missing Kafka end offset for partition %d", partition)
		}
	}

	return calculateKafkaLag(endOffsets, committedOffsets), nil
}

func calculateKafkaLag(endOffsets, committedOffsets map[int]int64) int64 {
	var lag int64
	for partition, endOffset := range endOffsets {
		committedOffset := committedOffsets[partition]
		if committedOffset < 0 {
			committedOffset = 0
		}
		if endOffset > committedOffset {
			lag += endOffset - committedOffset
		}
	}
	return lag
}

func waitUntilPipelineComplete(
	ctx context.Context,
	probe *pipelineProbe,
	workflowID string,
	baseline pipelineSnapshot,
	expected int64,
	started time.Time,
	httpElapsed time.Duration,
	pollInterval time.Duration,
) (pipelineSnapshot, pipelineMilestones, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	milestones := pipelineMilestones{HTTPComplete: httpElapsed}
	var latest pipelineSnapshot
	var lastProbeErr error

	for {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		snapshot, err := probe.Snapshot(probeCtx, workflowID)
		cancel()

		if err == nil {
			latest = snapshot
			lastProbeErr = nil
			elapsed := time.Since(started)
			observePipelineMilestones(&milestones, snapshot, baseline, expected, elapsed)

			if pipelineComplete(snapshot, baseline, expected) {
				milestones.EndToEnd = elapsed
				return snapshot, milestones, nil
			}
		} else {
			lastProbeErr = err
		}

		select {
		case <-ctx.Done():
			milestones.EndToEnd = time.Since(started)
			message := fmt.Sprintf(
				"timed out waiting for pipeline: records=%d/%d unpublished=%d notifications=%d/%d kafkaLag=%d",
				latest.Records-baseline.Records,
				expected,
				latest.Unpublished,
				latest.Notifications-baseline.Notifications,
				expected,
				latest.KafkaLag,
			)
			if lastProbeErr != nil {
				message += fmt.Sprintf(" lastProbeError=%v", lastProbeErr)
			}
			return latest, milestones, errors.New(message)
		case <-ticker.C:
		}
	}
}

func observePipelineMilestones(
	milestones *pipelineMilestones,
	snapshot pipelineSnapshot,
	baseline pipelineSnapshot,
	expected int64,
	elapsed time.Duration,
) {
	recordsComplete := snapshot.Records-baseline.Records >= expected
	notificationsComplete := snapshot.Notifications-baseline.Notifications >= expected

	if milestones.OutboxPublished == 0 && recordsComplete && snapshot.Unpublished == 0 {
		milestones.OutboxPublished = elapsed
	}
	if milestones.NotificationsComplete == 0 && notificationsComplete {
		milestones.NotificationsComplete = elapsed
	}
	if milestones.KafkaCaughtUp == 0 &&
		milestones.OutboxPublished > 0 &&
		snapshot.KafkaLag == 0 {
		milestones.KafkaCaughtUp = elapsed
	}
}

func pipelineComplete(snapshot, baseline pipelineSnapshot, expected int64) bool {
	return snapshot.Records-baseline.Records >= expected &&
		snapshot.Unpublished == 0 &&
		snapshot.Notifications-baseline.Notifications >= expected &&
		snapshot.KafkaLag == 0
}

func createWorkflow(client *http.Client, baseURL, token string) (string, error) {
	payload := map[string]any{
		"name":   fmt.Sprintf("Load Test %d", time.Now().Unix()),
		"states": []string{"SUBMITTED", "APPROVED"},
		"fields": []map[string]any{{
			"key":      "request",
			"label":    "Request",
			"type":     "text",
			"required": true,
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/workflows", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}
	var created workflowResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("workflow response did not contain an id")
	}
	return created.ID, nil
}

func createRecord(client *http.Client, baseURL, token, workflowID, runID string, index int) requestResult {
	payload := map[string]any{
		"workflowId": workflowID,
		"data": map[string]any{
			"request": fmt.Sprintf("Load request %d", index),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return requestResult{Err: err}
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/records", bytes.NewReader(body))
	if err != nil {
		return requestResult{Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("load-%s-%d", runID, index))

	started := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(started)
	if err != nil {
		return requestResult{Duration: duration, Err: err}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return requestResult{Duration: duration, Status: resp.StatusCode}
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func maxDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func splitCSV(value string) []string {
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func stageDelta(later, earlier time.Duration) string {
	if later <= 0 {
		return "not observed"
	}
	delta := later - earlier
	if delta < 0 {
		delta = 0
	}
	return delta.Round(time.Millisecond).String()
}

func formatAlignedDuration(value time.Duration) string {
	text := value.Round(time.Millisecond).String()
	if len(text) >= 5 {
		return " " + text
	}
	return strings.Repeat(" ", 6-len(text)) + text
}
