#!/bin/sh
# A stand-in stream-json agent CLI for tests: it ignores the prompt and emits a
# representative Claude Code event stream (init → tool_use → text → result) so
# Execute's parsing, usage extraction, and session-id capture can be verified
# without invoking a real (paid) agent.
cat <<'EOF'
{"type":"system","subtype":"init","session_id":"sess-xyz"}
{"type":"assistant","message":{"model":"m","content":[{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"a.go"}}]}}
{"type":"assistant","message":{"model":"m","content":[{"type":"text","text":"Done."}]}}
{"type":"result","subtype":"success","session_id":"sess-xyz","result":"Done.","modelUsage":{"m":{"inputTokens":120,"outputTokens":30}}}
EOF
