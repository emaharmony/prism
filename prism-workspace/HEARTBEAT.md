# Heartbeat — Lumi on Prism

## Purpose
Lightweight project awareness for heartbeat checks.

## Active Projects
- **Prism** — You ARE Prism. Running v28 with security layer, project tools, git tools, rich prompts.
- **Eggventura** — Pet Tycoon Roblox game. PT-002 bugs open.
- **Roblox Factory** — Multi-game pipeline. Runs on Snippy (Windows).

## Model Stack
- **Cloud:** glm-5.1:cloud (Lumi on Prism), deepseek-v4-pro:cloud (Mango)
- **Ollama:** cloud-only, no local LLMs

## Constraints
- Do NOT autonomously merge code, change architecture, or change workflows
- Do NOT push to GitHub without user approval (PRs only, approval system for git mutations)
- Git mutations require approval: git_add, git_commit, git_push
- Tag user at milestones or blockers only

## Idle Rule
If idle >20min with no active tasks: go to sleep. Wake on new message only.