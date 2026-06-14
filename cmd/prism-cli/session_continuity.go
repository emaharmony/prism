package main

import (
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/session"
)

type continuityWindow struct {
	recallStart time.Time
	weekStart   time.Time
}

func continuityLocation(cfg *orchestrator.Config) *time.Location {
	if cfg == nil || cfg.Sessions.RecallTimezone == "" || cfg.Sessions.RecallTimezone == "Local" {
		return time.Local
	}
	loc, err := time.LoadLocation(cfg.Sessions.RecallTimezone)
	if err != nil {
		return time.Local
	}
	return loc
}

func continuityWindowFor(cfg *orchestrator.Config, now time.Time) continuityWindow {
	loc := continuityLocation(cfg)
	mode := "calendar_week"
	if cfg != nil && cfg.Sessions.RecallWindowMode != "" {
		mode = cfg.Sessions.RecallWindowMode
	}
	localNow := now.In(loc)
	weekStartLocal := startOfWeek(localNow)
	weekStart := weekStartLocal.UTC()
	if mode == "rolling_days" {
		return continuityWindow{
			recallStart: now.UTC().Add(-continuityRecallWindow(cfg)),
			weekStart:   weekStart,
		}
	}
	return continuityWindow{recallStart: weekStart, weekStart: weekStart}
}

func startOfWeek(t time.Time) time.Time {
	year, month, day := t.Date()
	dayStart := time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	offset := (int(dayStart.Weekday()) + 6) % 7 // Monday = 0
	return dayStart.AddDate(0, 0, -offset)
}

func continuityRecallWindow(cfg *orchestrator.Config) time.Duration {
	if cfg == nil || cfg.Sessions.ShortTermWindowDays <= 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(cfg.Sessions.ShortTermWindowDays) * 24 * time.Hour
}

func continuityVerbatimLimit(cfg *orchestrator.Config) int {
	if cfg == nil || cfg.Sessions.VerbatimRecentMessages <= 0 {
		return 40
	}
	return cfg.Sessions.VerbatimRecentMessages
}

func getOrCreateSessionForMessage(mgr *session.Manager, cfg *orchestrator.Config, agentID, channel, channelID, externalUserID string) (*session.Session, string, error) {
	ownerID := externalUserID
	if cfg != nil {
		ownerID = cfg.ResolveOwnerID(channel, externalUserID)
	}
	if cfg != nil && cfg.Sessions.ContinuityScope == "channel_user" {
		sess, err := mgr.FindActive(channel, channelID, externalUserID)
		if err != nil || sess != nil {
			return sess, ownerID, err
		}
		sess, err = mgr.Create(agentID, channel, channelID, externalUserID)
		return sess, ownerID, err
	}
	aliases := []string{externalUserID}
	if cfg != nil {
		aliases = cfg.OwnerAliases(ownerID)
	}
	window := continuityWindowFor(cfg, time.Now())
	sess, err := mgr.GetOrCreateOwnerAgentSince(
		agentID,
		channel,
		channelID,
		ownerID,
		aliases,
		window.recallStart,
		window.weekStart,
		continuityVerbatimLimit(cfg),
	)
	return sess, ownerID, err
}

func cloneSessionWithSystemMemory(sess *session.Session, content string) *session.Session {
	if sess == nil || content == "" {
		return sess
	}
	cloned := *sess
	cloned.Messages = make([]session.Message, 0, len(sess.Messages)+1)
	cloned.Messages = append(cloned.Messages, session.Message{
		ID:        "remembrance-context",
		Role:      "system",
		Content:   content,
		Timestamp: time.Now().UTC(),
	})
	cloned.Messages = append(cloned.Messages, sess.Messages...)
	return &cloned
}
