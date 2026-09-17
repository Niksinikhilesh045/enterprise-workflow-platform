package main

import (
	"bytes"
	"encoding/json"
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
)

type workflowResponse struct {
	ID string `json:"id"`
}

type requestResult struct {
	Duration time.Duration
	Status   int
	Err      error
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "API base URL")
	token := flag.String("token", "", "bearer token with permission to create workflows and records")
	workflowID := flag.String("workflow-id", "", "existing workflow ID; when empty, the tool creates one")
	requests := flag.Int("requests", 1000, "number of record-create requests")
	concurrency := flag.Int("concurrency", 20, "number of concurrent workers")
	timeout := flag.Duration("timeout", 15*time.Second, "per-request timeout")
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
	elapsed := time.Since(started)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	throughput := float64(*requests) / elapsed.Seconds()

	fmt.Println("\nLoad test results")
	fmt.Printf("requests:     %d\n", *requests)
	fmt.Printf("concurrency:  %d\n", workerCount)
	fmt.Printf("successful:   %d\n", successful)
	fmt.Printf("failed:       %d\n", failed)
	fmt.Printf("elapsed:      %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("throughput:   %.2f req/s\n", throughput)
	fmt.Printf("latency p50:  %s\n", percentile(latencies, 0.50).Round(time.Microsecond))
	fmt.Printf("latency p95:  %s\n", percentile(latencies, 0.95).Round(time.Microsecond))
	fmt.Printf("latency p99:  %s\n", percentile(latencies, 0.99).Round(time.Microsecond))
	fmt.Printf("latency max:  %s\n", maxDuration(latencies).Round(time.Microsecond))
	fmt.Printf("status codes: %v\n", statusCounts)

	if failed > 0 {
		os.Exit(1)
	}
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
