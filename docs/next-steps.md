# Next steps

What's left after build steps 1 to 17, in the suggested order. The [build log](build-log.md) has the details behind each item.

## 1. Try the Mac cleanup by hand

`gk review mac` and `gk clean mac --apply` (step 15) have only run in tests. They move files to the Trash, so try them first, on something small:

1. `gk scan`
2. `gk review mac`: tick one or two small `safe` caches, then press enter to save.
3. `gk clean mac`: a dry run; check the count and total.
4. `gk clean mac --apply`: confirm, then check the items are in the Trash.

Record the result in the build log under step 15.

## 2. Open decisions

From the plan's "Open decisions":

- **MCP clients:** document Claude Desktop as a supported client, given that results go to Anthropic (including Downloads file names), or only local clients such as Ollama?
- **MCP SDK size:** the SDK adds 4 MB to `gk` (18.7 MB to 22.7 MB). Acceptable for a stdio-only server?
- **`review mac`:** start with the `safe` rows ticked (the plan starts with nothing ticked)? Is 7 days the right limit on the scan's age?
- **Reports:** keep both `latest.html` and a dated report per run, or only `latest.html`?

## 3. Small known issues

- **README status line** doesn't mention `review mac` and `clean mac` (step 15).
- **MCP time limit:** once the 45 seconds have passed, folders that don't exist are listed as `timeout`, because `size.Measure` checks the context before `WalkDir` reports the missing folder.
- **`gk logs`** reads only the current log file, not the rotated `.1` to `.3` files.
- **A 22-minute scan** with two timeouts was logged overnight during the MCP spike. The guess is that the Mac slept mid-scan; it wasn't proven.

## 4. Unverified on real data

These need the phone or the Photos library:

- `clean photos` on the Mac library hasn't run against a real library, so macOS's confirmation and Recently Deleted are unverified there.
- The iPhone's `no-device` and `locked` messages haven't been checked with the phone unplugged or locked.
- Whether deleting a Live Photo also removes its video, and whether deleting an edited original on the device also removes its render.

## 5. CI

There's no CI, so `make test` only runs on this Mac. A GitHub Actions workflow on a macOS runner would run it on every push. It's a separate decision, since it runs the tests on an outside service.
