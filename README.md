# ephemeral

Minimal, terminal-native git hosting. Single Go binary containing server, CLI, and TUI.

> **Pre-alpha software.** Not ready for production use.

## Quick Start (sprites.dev)

```bash
# Create a sprite (brings you into the console)
sprite create <sprite-name>

# Install and run
go install github.com/bantamhq/ephemeral/cmd/eph@latest
~/go/bin/eph serve

# Follow prompts, save your token, Ctrl+\ to detach

# Make publicly accessible
sprite url update --auth public -s <sprite-name>

# Login from your local machine
eph login <sprite-url>
```
