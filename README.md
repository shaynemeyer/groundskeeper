<p align="center">
  <img src="assets/logo-1.jpg" alt="Groundskeeper logo: Rook the raccoon in a hard hat" width="320">
</p>

<p align="center">Keep your Mac's disk, and a connected iPhone, free of clutter.</p>

> **Status:** early development. `scan` covers every Mac scanner in the plan, `make install` schedules it for Monday and Thursday mornings with a notification, `clean browsers` moves browser caches to the Trash, and `gk doctor` checks the setup. `gk iphone` reports on a phone connected over USB, `gk review photos` steps through its duplicate photos, and `clean photos` deletes the ones you approve. `gk mcp` lets an AI assistant read the results. See the [implementation plan](docs/groundskeeper-plan.md) for what's next.

Groundskeeper (`groundskeeper`, alias `gk`) is a terminal tool for macOS. Twice a week it scans for space you can reclaim and writes an HTML report. It looks for:

- caches
- AI models (Ollama, Hugging Face, LM Studio)
- stale build artifacts
- toolchains
- containers
- old downloads
- browser caches
- unused apps

With an iPhone plugged in over USB, it also reports on the phone's apps, shared files and duplicate photos.

## How it stays safe

- **Scans are read-only.** The report only suggests commands. Nothing is deleted unless you run a `clean` command.
- **`clean` is a dry run unless you add `--apply`.** Browser caches go to the Trash. Photos deleted from the Mac library go to Recently Deleted; photos deleted from the iPhone over USB are gone for good, and the command says so first. Site data, cookies and passwords are never touched.
- **Nothing leaves your Mac** unless you connect a cloud AI assistant (below). It makes no network requests, measures file sizes without reading contents, and keeps its reports and logs private to your user.

## Get started

Groundskeeper needs macOS 26, the Xcode Command Line Tools and Go. Build and install it from source:

```sh
git clone https://github.com/shaynemeyer/groundskeeper.git
cd groundskeeper
make install
gk doctor
```

The [user guide](docs/user-guide.md) covers the rest: Full Disk Access, reading the report, cleaning up, the schedule, the iPhone and photos, and privacy. `gk help <command>` lists every option.

## Ask an AI assistant

`gk mcp` serves three read-only [MCP](https://modelcontextprotocol.io) tools, `scan`, `history` and `stats`, so an assistant can answer "what's using my disk?". It can't clean anything.

**Privacy:** with Claude Desktop, results go to Anthropic, including paths that name your projects, apps and Downloads files. With a local Ollama model they stay on your Mac. Either way, `~/bin/groundskeeper` needs Full Disk Access itself; the assistant app's access doesn't count.

- **Claude Desktop:** in Settings, Developer, Edit Config, add this to `claude_desktop_config.json`, with the full path that `echo ~/bin/groundskeeper` prints. Then quit and reopen Claude Desktop.

  ```json
  { "mcpServers": { "groundskeeper": { "command": "<the full path>", "args": ["mcp"] } } }
  ```

- **Ollama:** save the same setting as `~/.config/groundskeeper/mcp.json`, then run [`ollmcp`](https://github.com/jonigl/mcp-client-for-ollama) with a model that can use tools:

  ```sh
  uvx --from mcp-client-for-ollama ollmcp -j ~/.config/groundskeeper/mcp.json -m qwen3.8:27b-mlx
  ```

The [user guide](docs/user-guide.md#ask-an-ai-assistant) has more.

## Development

```sh
make build                        # bin/groundskeeper and bin/photos-helper
make test                         # go test ./... and govulncheck
make run                          # scan and open the report
./bin/groundskeeper scan --dry    # summary only, writes nothing
```

## Docs

- [User guide](docs/user-guide.md): installing and using Groundskeeper, task by task
- [Implementation plan](docs/groundskeeper-plan.md): architecture, scanners, security, logging and build order
- [Rook design brief](docs/rook-design-brief.md): the mascot, palette and voice
- [Build log](docs/build-log.md): what each build step delivered, decisions and open items
