// photos-helper reads photos for Groundskeeper and answers with JSON on stdout.
// Usage:
//   photos-helper list --source device
//   photos-helper analyze --source device < ids.json
import Foundation

let usage = """
    usage: photos-helper list --source device
           photos-helper analyze --source device < ids.json

    """

enum Command: String {
    case list, analyze
}

func usageError() -> Never {
    FileHandle.standardError.write(Data(usage.utf8))
    exit(2)
}

let args = Array(CommandLine.arguments.dropFirst())
guard args.count == 3, args[1...] == ["--source", "device"], let command = Command(rawValue: args[0]) else {
    usageError()
}
let ids = command == .analyze ? readIDs() : []

let catalog = DeviceCatalog { result in
    guard case .success(let files) = result else {
        if case .failure(let kind) = result { fail(kind) }
        return
    }
    switch command {
    case .list: emit(Listing(source: "device", assets: files.map(Asset.init)))
    case .analyze: Task { emit(await analyze(ids, in: files)) }
    }
}
catalog.start()
RunLoop.main.run()

/// Reads a JSON array of asset IDs from stdin.
func readIDs() -> [String] {
    let data = FileHandle.standardInput.readDataToEndOfFile()
    guard let ids = try? JSONDecoder().decode([String].self, from: data) else { usageError() }
    return ids
}
