"""Block process-name kills in automated repair sessions on shared runners.

This is a conservative command-text guard, not a shell sandbox. It also
rejects names in arguments or nested shell strings. Inspect such text with
Read instead; terminate only processes the session started, by their PID.
"""

import json
import re
import sys


def main():
    try:
        event = json.load(sys.stdin)
        command = event["tool_input"]["command"]
        if event["tool_name"] != "Bash" or not isinstance(command, str):
            raise ValueError("expected a Bash command")
    except (ValueError, KeyError, TypeError) as error:
        print(f"Process guard could not read the tool input: {error}", file=sys.stderr)
        return 2

    # Normalize ordinary quoting, escaping and line continuations too. Scan
    # the whole call so a preceding command or nested shell cannot hide it.
    text = re.sub(r"\\\r?\n|[\\'\"]", "", command)
    if re.search(r"(?<![\w-])(?:pkill|killall)(?![\w-])", text):
        print(
            "Process-name kills are forbidden on shared runners. Use timeout "
            "when starting a reproduction, or terminate only a PID you started. "
            "Do not bypass this guard with another shell or signaling API.",
            file=sys.stderr,
        )
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
