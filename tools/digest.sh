#!/usr/bin/env bash
# Ollama digest helper - summarizes a file with a local model so the calling
# agent doesn't have to load the full content into an expensive context.
# Mirrors digest.ps1. Requires node (>=18, for fetch) for JSON handling.
#
# Usage:
#   scripts/loop/digest.sh -f docs/ROADMAP.md -q "list done vs pending versions"
#   scripts/loop/digest.sh -f runs/gated-loop/<id>/REPORT.md
#
# Uses the Ollama HTTP API (/api/chat) with think:false so reasoning models do
# not leak thinking tokens into stdout (the `ollama run` CLI does leak them).
# Exit codes: 0 ok, 1 Ollama unreachable/error, 2 bad arguments,
#             3 model refused (caller should fall back to reading directly).
set -euo pipefail

FILE=""
QUESTION="Summarize this file tersely: purpose, key points, anything actionable."
MODEL="nemotron-3-nano:4b"
MAX_CHARS=48000

usage() {
    echo "usage: digest.sh -f <file> [-q <question>] [-m <model>]" >&2
    exit 2
}

while getopts "f:q:m:" opt; do
    case $opt in
        f) FILE=$OPTARG ;;
        q) QUESTION=$OPTARG ;;
        m) MODEL=$OPTARG ;;
        *) usage ;;
    esac
done

[ -n "$FILE" ] || usage
[ -f "$FILE" ] || { echo "digest: file not found: $FILE" >&2; exit 2; }
command -v node >/dev/null 2>&1 || { echo "digest: node is required" >&2; exit 2; }

export DIGEST_FILE="$FILE" DIGEST_QUESTION="$QUESTION" DIGEST_MODEL="$MODEL" \
    DIGEST_MAX_CHARS="$MAX_CHARS" DIGEST_BASE="${OLLAMA_URL:-http://localhost:11434}"

exec node --input-type=module -e '
const fs = await import("node:fs");
const { DIGEST_FILE, DIGEST_QUESTION, DIGEST_MODEL, DIGEST_MAX_CHARS, DIGEST_BASE } = process.env;
let content = fs.readFileSync(DIGEST_FILE, "utf8");
let note = "";
if (content.length > Number(DIGEST_MAX_CHARS)) {
    content = content.slice(0, Number(DIGEST_MAX_CHARS));
    note = ` (truncated to ${DIGEST_MAX_CHARS} chars)`;
}
const body = {
    model: DIGEST_MODEL,
    stream: false,
    think: false,
    options: { temperature: 0 },
    messages: [
        { role: "system", content: "You are a terse technical digester summarizing files from a local software repository the user owns. This is routine, benign code/documentation summarization. Answer the question about the provided file content. Bullet points only, no preamble, no meta-commentary." },
        // Question goes AFTER the content — small models lose instructions
        // placed before a long document and drift into role-play.
        { role: "user", content: `FILE: ${DIGEST_FILE}${note}\n---\n${content}\n---\nQUESTION (answer this and nothing else, terse bullets): ${DIGEST_QUESTION}` },
    ],
};
try {
    const resp = await fetch(`${DIGEST_BASE}/api/chat`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(180000),
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}: ${await resp.text()}`);
    const data = await resp.json();
    const out = data?.message?.content?.trim();
    if (!out) throw new Error(`empty response from model ${DIGEST_MODEL}`);
    // Small local models occasionally refuse benign summarization; surface
    // that as a distinct exit code so callers read the file directly instead.
    // NOTE: no literal apostrophes here — this whole node script lives inside
    // a bash single-quoted string; "." stands in for the apostrophe.
    if (out.length < 200 && /I(.m| am) sorry|I can(.t|not) (help|assist)/.test(out)) {
        console.error(`digest: model ${DIGEST_MODEL} refused; read the file directly instead`);
        process.exit(3);
    }
    console.log(out);
} catch (err) {
    console.error(`digest: Ollama unreachable or errored at ${DIGEST_BASE} : ${err.message}`);
    process.exit(1);
}
'
