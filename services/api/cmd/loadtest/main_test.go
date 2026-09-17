package main

import "testing"

func TestCalculateKafkaLag(t *testing.T) {
	end := map[int]int64{0: 100, 1: 75, 2: 50}
	committed := map[int]int64{0: 100, 1: 70, 2: 40}

	if got, want := calculateKafkaLag(end, committed), int64(15); got != want {
		t.Fatalf("calculateKafkaLag() = %d, want %d", got, want)
	}
}

func TestCalculateKafkaLagTreatsUncommittedAsZero(t *testing.T) {
	end := map[int]int64{0: 10}
	committed := map[int]int64{0: -1}

	if got, want := calculateKafkaLag(end, committed), int64(10); got != want {
		t.Fatalf("calculateKafkaLag() = %d, want %d", got, want)
	}
}

func TestPipelineComplete(t *testing.T) {
	baseline := pipelineSnapshot{Records: 10, Notifications: 10}
	complete := pipelineSnapshot{
		Records:       5010,
		Unpublished:   0,
		Notifications: 5010,
		KafkaLag:      0,
	}
	if !pipelineComplete(complete, baseline, 5000) {
		t.Fatal("expected pipeline to be complete")
	}

	cases := []struct {
		name     string
		snapshot pipelineSnapshot
	}{
		{
			name: "record shortfall",
			snapshot: pipelineSnapshot{
				Records:       5009,
				Notifications: 5010,
			},
		},
		{
			name: "unpublished outbox",
			snapshot: pipelineSnapshot{
				Records:       5010,
				Unpublished:   1,
				Notifications: 5010,
			},
		},
		{
			name: "notification shortfall",
			snapshot: pipelineSnapshot{
				Records:       5010,
				Notifications: 5009,
			},
		},
		{
			name: "consumer lag",
			snapshot: pipelineSnapshot{
				Records:       5010,
				Notifications: 5010,
				KafkaLag:      1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if pipelineComplete(tc.snapshot, baseline, 5000) {
				t.Fatal("expected pipeline to be incomplete")
			}
		})
	}
}
