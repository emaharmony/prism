package main

import "github.com/emaharmony/prizm/internal/orchestrator"

// defaultConversationPostfix shapes an agent's default conversational tone
// when neither the agent's own conversation_postfix nor a channel's
// Personality directive apply. Keep this reasonably neutral — anything more
// opinionated here becomes the tone every agent gets whenever an operator
// hasn't explicitly configured one.
const defaultConversationPostfix = "Stay present in the conversation. Ask follow-up questions when appropriate. " +
	"Don't wrap things up unless the topic is genuinely resolved. " +
	"Be warm, curious, and engaged — not a transactional Q&A machine."

// resolveConversationPostfix picks the "How You Respond" directive for a
// message, in priority order:
//  1. The agent's own explicit conversation_postfix, if set — an operator's
//     deliberate agent-level choice always wins.
//  2. The resolved channel's Personality directive, if the channel role sets
//     one and the agent has no explicit postfix — lets a channel override the
//     harness's own generic default without touching agent config.
//  3. defaultConversationPostfix, if neither of the above apply.
func resolveConversationPostfix(agentCfg *orchestrator.AgentConfig, channelRole *orchestrator.ChannelRole) string {
	if agentCfg != nil && agentCfg.ConversationPostfix != "" {
		return agentCfg.ConversationPostfix
	}
	if channelRole != nil {
		if directive := orchestrator.PersonalityDirective(channelRole.Personality); directive != "" {
			return directive
		}
	}
	return defaultConversationPostfix
}
