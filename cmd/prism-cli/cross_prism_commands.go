package main

import (
	ctxcontext "context"
	"fmt"
	"log"
	"strings"

	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/crossprism"
)

func (cc *conversationContext) handleCrossPrismCommand(msg *discordbot.InboundMessage) bool {
	command, rest, ok := parsePrismCommand(msg.Content)
	if !ok {
		return false
	}

	switch command {
	case "delegate":
		cc.handleCrossPrismDelegate(msg, rest)
	case "status":
		cc.handleCrossPrismStatus(msg, rest)
	case "stop", "cancel":
		cc.handleCrossPrismStop(msg, rest)
	default:
		cc.sendCrossPrismCommandReply(msg.ChannelID, "Unknown Prism command. Use `/prism delegate`, `/prism status`, or `/prism stop`.")
	}
	return true
}

func (cc *conversationContext) handleCrossPrismDelegate(msg *discordbot.InboundMessage, rest string) {
	args, taskText := parseCrossPrismArgs(rest)
	if taskText == "" {
		cc.sendCrossPrismCommandReply(msg.ChannelID, "Missing task. Example: `/prism delegate target:factory task:run a Factory smoke check and report artifacts`")
		return
	}
	targetProfile := firstArg(args, "target", "profile")
	if targetProfile == "" {
		targetProfile = "generic"
	}

	req := crossprism.DelegateRequest{
		TargetProfile:  targetProfile,
		TargetInstance: firstArg(args, "to", "instance"),
		LeaderInstance: firstArg(args, "leader"),
		Task:           taskText,
		Priority:       firstArg(args, "priority"),
		Context: map[string]any{
			"source":     "discord_command",
			"channel_id": msg.ChannelID,
			"user_id":    msg.UserID,
			"user_name":  msg.UserName,
		},
	}
	sent, err := cc.crossCoord.Delegate(ctxcontext.Background(), req)
	if err != nil {
		log.Printf("[CROSS-PRISM] delegate command failed: %v", err)
		cc.sendCrossPrismCommandReply(msg.ChannelID, fmt.Sprintf("Cross-Prism delegation failed: %v", err))
		return
	}
	cc.sendCrossPrismCommandReply(msg.ChannelID, fmt.Sprintf("Delegated over NATS to `%s` using `%s` profile. Thread `%s`.", sent.To, targetProfile, sent.CorrelationID))
}

func (cc *conversationContext) handleCrossPrismStatus(msg *discordbot.InboundMessage, rest string) {
	args, taskValue := parseCrossPrismArgs(rest)
	taskID := firstNonEmptyCommandArg(firstArg(args, "task", "task_id", "thread", "thread_id"), taskValue)
	target := crossPrismTarget(cc, args)
	if taskID == "" || target == "" {
		cc.sendCrossPrismCommandReply(msg.ChannelID, "Missing target or task id. Example: `/prism status target:generic task:cross-corr-abc123`")
		return
	}
	sent, err := cc.crossCoord.RequestStatus(target, taskID)
	if err != nil {
		log.Printf("[CROSS-PRISM] status command failed: %v", err)
		cc.sendCrossPrismCommandReply(msg.ChannelID, fmt.Sprintf("Cross-Prism status request failed: %v", err))
		return
	}
	cc.sendCrossPrismCommandReply(msg.ChannelID, fmt.Sprintf("Requested status from `%s` for `%s`. Correlation `%s`.", target, taskID, sent.CorrelationID))
}

func (cc *conversationContext) handleCrossPrismStop(msg *discordbot.InboundMessage, rest string) {
	args, taskValue := parseCrossPrismArgs(rest)
	taskID := firstNonEmptyCommandArg(firstArg(args, "task", "task_id", "thread", "thread_id"), taskValue)
	target := crossPrismTarget(cc, args)
	if taskID == "" || target == "" {
		cc.sendCrossPrismCommandReply(msg.ChannelID, "Missing target or task id. Example: `/prism stop target:generic task:cross-corr-abc123`")
		return
	}
	sent, err := cc.crossCoord.Cancel(target, taskID)
	if err != nil {
		log.Printf("[CROSS-PRISM] stop command failed: %v", err)
		cc.sendCrossPrismCommandReply(msg.ChannelID, fmt.Sprintf("Cross-Prism stop request failed: %v", err))
		return
	}
	cc.sendCrossPrismCommandReply(msg.ChannelID, fmt.Sprintf("Sent stop request to `%s` for `%s`. Correlation `%s`.", target, taskID, sent.CorrelationID))
}

func (cc *conversationContext) sendCrossPrismCommandReply(channelID, content string) {
	if cc.bot == nil {
		return
	}
	if err := cc.bot.Send(&discordbot.OutboundMessage{ChannelID: channelID, Content: content}); err != nil {
		log.Printf("[CROSS-PRISM] failed to send command reply: %v", err)
	}
}

func parsePrismCommand(content string) (string, string, bool) {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)
	var rest string
	switch {
	case strings.HasPrefix(lower, "/prism "):
		rest = strings.TrimSpace(trimmed[len("/prism "):])
	case lower == "/prism":
		return "", "", true
	case strings.HasPrefix(lower, "prism "):
		rest = strings.TrimSpace(trimmed[len("prism "):])
	case lower == "prism":
		return "", "", true
	default:
		return "", "", false
	}
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return "", "", true
	}
	command := strings.ToLower(parts[0])
	return command, strings.TrimSpace(rest[len(parts[0]):]), true
}

func parseCrossPrismArgs(rest string) (map[string]string, string) {
	args := map[string]string{}
	taskText := ""
	fields := strings.Fields(rest)
	for i := 0; i < len(fields); i++ {
		part := fields[i]
		if strings.HasPrefix(strings.ToLower(part), "task:") {
			value := strings.TrimPrefix(part, part[:len("task:")])
			remaining := []string{}
			if value != "" {
				remaining = append(remaining, value)
			}
			remaining = append(remaining, fields[i+1:]...)
			taskText = strings.TrimSpace(strings.Join(remaining, " "))
			break
		}
		key, value, ok := strings.Cut(part, ":")
		if ok {
			args[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	return args, taskText
}

func crossPrismTarget(cc *conversationContext, args map[string]string) string {
	if target := firstArg(args, "to", "instance"); target != "" {
		return target
	}
	profileName := firstArg(args, "target", "profile")
	if profileName == "" {
		profileName = "generic"
	}
	if profile, ok := cc.crossCoord.ResolveProfile(profileName); ok {
		return profile.InstanceID
	}
	return ""
}

func firstArg(args map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(args[strings.ToLower(key)]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyCommandArg(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
