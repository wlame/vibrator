#!/usr/bin/env python3
"""
Langfuse Stop Hook for Claude Code
Sends session data to Langfuse running on host for observability and analytics.
Based on: https://github.com/doneyli/claude-code-langfuse-template
"""

import os
import sys
import json
import requests
from pathlib import Path
from datetime import datetime

# Configuration from environment
TRACE_TO_LANGFUSE = os.getenv("TRACE_TO_LANGFUSE", "false").lower() == "true"
LANGFUSE_HOST = os.getenv("LANGFUSE_HOST", "http://host.docker.internal:3050")
LANGFUSE_PUBLIC_KEY = os.getenv("LANGFUSE_PUBLIC_KEY", "")
LANGFUSE_SECRET_KEY = os.getenv("LANGFUSE_SECRET_KEY", "")
DEBUG = os.getenv("CC_LANGFUSE_DEBUG", "false").lower() == "true"

def debug_log(msg):
    """Log debug messages if debug mode is enabled."""
    if DEBUG:
        print(f"[Langfuse Hook] {msg}", file=sys.stderr)

def send_to_langfuse(transcript_path):
    """Send the latest session data to Langfuse."""
    if not TRACE_TO_LANGFUSE:
        debug_log("Tracing disabled (TRACE_TO_LANGFUSE=false)")
        return

    if not LANGFUSE_PUBLIC_KEY or not LANGFUSE_SECRET_KEY:
        debug_log("Langfuse keys not configured, skipping trace")
        return

    # Read the transcript file
    try:
        with open(transcript_path, 'r') as f:
            lines = f.readlines()
            if not lines:
                debug_log("Empty transcript file")
                return

            # Get the last message (the one that just completed)
            last_line = lines[-1]
            message = json.loads(last_line)

            # Extract session info from path
            session_id = Path(transcript_path).stem
            project_name = Path(transcript_path).parent.name

            # Prepare trace data
            trace_data = {
                "id": f"{session_id}_{len(lines)}",
                "name": f"claude_session_{session_id}",
                "sessionId": session_id,
                "metadata": {
                    "project": project_name,
                    "message_index": len(lines)
                },
                "input": message.get("content", ""),
                "output": message.get("content", "") if message.get("role") == "assistant" else "",
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

            # Send to Langfuse
            headers = {
                "Content-Type": "application/json",
                "Authorization": f"Basic {LANGFUSE_PUBLIC_KEY}:{LANGFUSE_SECRET_KEY}"
            }

            response = requests.post(
                f"{LANGFUSE_HOST}/api/public/traces",
                headers=headers,
                json=trace_data,
                timeout=2
            )

            if response.status_code in [200, 201]:
                debug_log(f"Trace sent successfully: {session_id}")
            else:
                debug_log(f"Failed to send trace: {response.status_code} - {response.text}")

    except FileNotFoundError:
        debug_log(f"Transcript file not found: {transcript_path}")
    except Exception as e:
        debug_log(f"Error sending to Langfuse: {e}")

def main():
    """Main entry point for the stop hook."""
    # Get transcript path from Claude Code
    # Format: ~/.claude/projects/<project>/<session>.jsonl
    transcript_path = os.getenv("CLAUDE_TRANSCRIPT_PATH")

    if not transcript_path:
        debug_log("No CLAUDE_TRANSCRIPT_PATH provided")
        return

    debug_log(f"Processing transcript: {transcript_path}")
    send_to_langfuse(transcript_path)

if __name__ == "__main__":
    main()
