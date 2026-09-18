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


func TestObservePipelineMilestonesOrdersKafkaAfterOutbox(t *testing.T) {
	baseline := pipelineSnapshot{}
	milestones := pipelineMilestones{HTTPComplete: 5_000_000_000}

	observePipelineMilestones(&milestones, pipelineSnapshot{
		Records:       5000,
		Unpublished:   100,
		Notifications: 4900,
		KafkaLag:      0,
	}, baseline, 5000, 6_000_000_000)

	if milestones.KafkaCaughtUp != 0 {
		t.Fatalf("Kafka milestone recorded before outbox completion: %s", milestones.KafkaCaughtUp)
	}

	observePipelineMilestones(&milestones, pipelineSnapshot{
		Records:       5000,
		Unpublished:   0,
		Notifications: 4950,
		KafkaLag:      2,
	}, baseline, 5000, 7_000_000_000)

	if milestones.OutboxPublished != 7_000_000_000 {
		t.Fatalf("outbox milestone = %s, want 7s", milestones.OutboxPublished)
	}
	if milestones.KafkaCaughtUp != 0 {
		t.Fatalf("Kafka milestone recorded while lag was non-zero: %s", milestones.KafkaCaughtUp)
	}

	observePipelineMilestones(&milestones, pipelineSnapshot{
		Records:       5000,
		Unpublished:   0,
		Notifications: 5000,
		KafkaLag:      0,
	}, baseline, 5000, 8_000_000_000)

	if milestones.KafkaCaughtUp != 8_000_000_000 {
		t.Fatalf("Kafka milestone = %s, want 8s", milestones.KafkaCaughtUp)
	}
	if milestones.NotificationsComplete != 8_000_000_000 {
		t.Fatalf("notification milestone = %s, want 8s", milestones.NotificationsComplete)
	}
}

func TestObservePipelineMilestonesKeepsFirstObservation(t *testing.T) {
	milestones := pipelineMilestones{}

	complete := pipelineSnapshot{
		Records:       5000,
		Notifications: 5000,
		KafkaLag:      0,
	}
	observePipelineMilestones(&milestones, complete, pipelineSnapshot{}, 5000, 9_000_000_000)
	observePipelineMilestones(&milestones, complete, pipelineSnapshot{}, 5000, 10_000_000_000)

	if milestones.OutboxPublished != 9_000_000_000 {
		t.Fatalf("outbox milestone overwritten: %s", milestones.OutboxPublished)
	}
	if milestones.KafkaCaughtUp != 9_000_000_000 {
		t.Fatalf("Kafka milestone overwritten: %s", milestones.KafkaCaughtUp)
	}
	if milestones.NotificationsComplete != 9_000_000_000 {
		t.Fatalf("notification milestone overwritten: %s", milestones.NotificationsComplete)
	}
}
