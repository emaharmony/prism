package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	url := os.Args[1]
	target := os.Args[2] // scout | chisel
	taskID := os.Args[3]
	desc := os.Args[4]
	// If desc is "@path", read the description from that file.
	if len(desc) > 1 && desc[0] == '@' {
		b, err := os.ReadFile(desc[1:])
		if err != nil {
			fmt.Println("read desc file error:", err)
			os.Exit(1)
		}
		desc = string(b)
	}

	nc, err := nats.Connect(url)
	if err != nil {
		fmt.Println("connect error:", err)
		os.Exit(1)
	}
	defer nc.Close()

	packet := map[string]any{
		"type":                 "task_delegation",
		"target_agent":         target,
		"task_id":              taskID,
		"description":          desc,
		"expected_deliverable": "Per spec in description.",
		"priority":             "normal",
		"required_capability":  "report",
	}
	if target == "chisel" {
		packet["required_capability"] = "code"
	}

	done := make(chan string, 1)
	_, _ = nc.Subscribe("prism.workflow.task.complete", func(m *nats.Msg) {
		var c map[string]any
		if json.Unmarshal(m.Data, &c) == nil {
			if c["task_id"] == taskID {
				done <- string(m.Data)
			}
		}
	})

	data, _ := json.Marshal(packet)
	if err := nc.Publish("prism.agent.openclaw", data); err != nil {
		fmt.Println("publish error:", err)
		os.Exit(1)
	}
	nc.Flush()
	fmt.Println("PUBLISHED target=" + target + " task_id=" + taskID)

	select {
	case res := <-done:
		fmt.Println("COMPLETION: " + res)
	case <-time.After(18 * time.Minute):
		fmt.Println("TIMEOUT (task may still be running; check serve log)")
	}
}
