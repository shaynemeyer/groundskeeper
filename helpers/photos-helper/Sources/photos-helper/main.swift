// photos-helper reads photos for Groundskeeper and answers with JSON on stdout.
// Usage:
//   photos-helper list --source device
//   photos-helper analyze --source device < ids.json
//   photos-helper thumbnail --source device --out <dir> < ids.json
import Foundation

let usage = """
    usage: photos-helper list --source device
           photos-helper analyze --source device < ids.json
           photos-helper thumbnail --source device --out <dir> < ids.json

    """

enum Command: String {
    case list, analyze, thumbnail
}

func usageError() -> Never {
    FileHandle.standardError.write(Data(usage.utf8))
    exit(2)
}

/// Parses "--name value" pairs. Returns nil for anything else.
func parseFlags(_ args: ArraySlice<String>) -> [String: String]? {
    guard args.count % 2 == 0 else { return nil }
    var flags: [String: String] = [:]
    for i in stride(from: args.startIndex, to: args.endIndex, by: 2) {
        guard args[i].hasPrefix("--") else { return nil }
        flags[String(args[i].dropFirst(2))] = args[i + 1]
    }
    return flags
}

/// Reads a JSON array of asset IDs from stdin.
func readIDs() -> [String] {
    let data = FileHandle.standardInput.readDataToEndOfFile()
    guard let ids = try? JSONDecoder().decode([String].self, from: data) else { usageError() }
    return ids
}

/// Creates dir with mode 0700 if it doesn't exist.
func outputDir(_ path: String) -> URL {
    let dir = URL(filePath: path)
    guard (try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])) != nil else {
        usageError()
    }
    return dir
}

let args = CommandLine.arguments.dropFirst()
guard let command = args.first.flatMap(Command.init), let flags = parseFlags(args.dropFirst()),
    flags["source"] == "device"
else { usageError() }
let allowed: Set<String> = command == .thumbnail ? ["source", "out"] : ["source"]
guard Set(flags.keys) == allowed else { usageError() }
let ids = command == .list ? [] : readIDs()
let out = flags["out"].map(outputDir)

let catalog = DeviceCatalog { result in
    guard case .success(let files) = result else {
        if case .failure(let kind) = result { fail(kind) }
        return
    }
    switch command {
    case .list: emit(Listing(source: "device", assets: files.map(Asset.init)))
    case .analyze: Task { emit(await analyze(ids, in: files)) }
    case .thumbnail: Task { emit(await writeThumbnails(ids, in: files, to: out!)) }
    }
}
catalog.start()
RunLoop.main.run()
