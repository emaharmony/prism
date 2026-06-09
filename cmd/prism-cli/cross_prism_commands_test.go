package main

import "testing"

func TestParsePrismCommand(t *testing.T) {
	command, rest, ok := parsePrismCommand("/prism delegate target:factory task:run smoke checks")
	if !ok {
		t.Fatal("expected command")
	}
	if command != "delegate" {
		t.Fatalf("command = %q", command)
	}
	if rest != "target:factory task:run smoke checks" {
		t.Fatalf("rest = %q", rest)
	}
}

func TestParseCrossPrismArgs(t *testing.T) {
	args, taskText := parseCrossPrismArgs("target:factory leader:lumi-ceo priority:high task:run smoke checks and report artifacts")
	if args["target"] != "factory" {
		t.Fatalf("target = %q", args["target"])
	}
	if args["leader"] != "lumi-ceo" {
		t.Fatalf("leader = %q", args["leader"])
	}
	if args["priority"] != "high" {
		t.Fatalf("priority = %q", args["priority"])
	}
	if taskText != "run smoke checks and report artifacts" {
		t.Fatalf("taskText = %q", taskText)
	}
}

func TestParseCrossPrismStatusTaskID(t *testing.T) {
	args, taskValue := parseCrossPrismArgs("target:generic task:cross-corr-abc123")
	taskID := firstNonEmptyCommandArg(firstArg(args, "task", "task_id"), taskValue)
	if taskID != "cross-corr-abc123" {
		t.Fatalf("taskID = %q", taskID)
	}
}
