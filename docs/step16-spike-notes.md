# Step 16 spike: where things stand

Working notes for the MCP spike (plan, "MCP server (opt-in)", "Spike before building"). Delete this file once the answers are in `build-log.md`.

## What's set up

- **Spike server** `cmd/mcpspike` (throwaway, delete after the spike). Tools: `scan` (runs the real scanners, returns items with paths and hints written as `~`, optional `top` per section) and `wait` (sleeps `seconds`, for timeouts). Every call is logged with timings to the file in `SPIKE_LOG`.
  - `SPIKE_WAIT_ONLY=1` offers only `wait`, and runs one scan in the background at startup that logs only the Full Disk Access probe and skip counts. Nothing about the Mac reaches the client.
- **Spike client** `cmd/mcpspikeclient` (throwaway): lists tools, calls `wait 1`, `scan top=5`, `scan`, and prints timings, sizes and whether a result holds the home path.
- **`go.mod` and `go.sum`** now require `github.com/modelcontextprotocol/go-sdk` v1.8.0 (latest release). Keep it for step 16 (2).
- **Claude Desktop** config has one added entry, `groundskeeper-spike`, with `SPIKE_WAIT_ONLY=1` and `SPIKE_LOG=$SPIKE/spike-desktop.log`. The original config is backed up as `$SPIKE/claude_desktop_config.backup.json`.
- **ollmcp** server file: `$SPIKE/ollmcp-servers.json` (full `scan`, logs to `$SPIKE/spike-ollama.log`).

`$SPIKE` is the session scratch folder under `/private/tmp`, which macOS may clear on restart. If it's gone, rebuild into any folder and point the Desktop entry and the ollmcp file at it:

```
go build -o "$SPIKE/mcpspike" ./cmd/mcpspike
go build -o "$SPIKE/mcpspikeclient" ./cmd/mcpspikeclient
SPIKE_LOG="$SPIKE/check.log" "$SPIKE/mcpspikeclient" "$SPIKE/mcpspike"
```

## Privacy decision

Real scan results are not sent to Anthropic during the spike. Claude Desktop gets only `wait` (questions 1 and 2); the full `scan` runs only through Ollama (question 3), where nothing leaves the Mac. Reason: results expose names even without contents, such as Downloads file names (the Downloads label is the file name), project folders and installed apps. Whether Anthropic keeps or trains on conversations depends on the account's privacy settings ([consumer terms update](https://www.anthropic.com/news/updates-to-our-consumer-terms), [Privacy Center](https://privacy.anthropic.com)).

## Found so far

- **Baseline from the Claude Code shell** (no Full Disk Access): a full scan took about 6 s and returned 7.2 KB with 32 items; top 5 per section returned 5.5 KB with 21 items. 26 paths were skipped for permission; probe `fda=missing`.
- **Hints leaked the user name.** Scanners build hints from absolute paths (`internal/scan/artifacts.go:171`, `rm -rf '/Users/...'`), so rewriting only `Path` as `~` isn't enough. The real `gk mcp` must rewrite hints too and test it.
- **`~` inside quotes doesn't expand.** Replacing the home folder inside a shell-quoted hint gives `'~/a b'`, which doesn't work when pasted. The real tool must write `~/'a b'`.
- **Readonly marking works.** The SDK client sees `readOnlyHint: true` on both tools.

## Still to do

1. **Claude Desktop:** quit it (Cmd-Q), reopen, wait about 15 s. Ask it to call groundskeeper-spike's `wait` with `seconds` 30, then 90, then 240. Note which fail and the error text. Check whether Claude.app has Full Disk Access in System Settings > Privacy & Security.
2. **Ollama,** in Ghostty:
   ```
   uvx --from mcp-client-for-ollama ollmcp -j "$SPIKE/ollmcp-servers.json" -m qwen3.8:27b-mlx
   ```
   Ask "What's using my disk? Use the scan tool." Repeat with `-m llama3.2`. Note whether each called the tool and whether the answer matched the data.
3. Read `spike-desktop.log` and `spike-ollama.log`, answer the three questions in `build-log.md`, and update the plan (hint rewriting, `~/'...'` quoting, the Claude Desktop open decision).
4. **Clean up:** restore the Desktop config from the backup (or remove the `groundskeeper-spike` entry), delete `cmd/mcpspike`, `cmd/mcpspikeclient` and this file.
5. Then step 16 (2): `gk mcp` with the `scan` tool, tested with `mcp.NewInMemoryTransports`.
