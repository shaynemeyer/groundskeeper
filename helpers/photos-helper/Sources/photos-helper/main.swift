// photos-helper reads photos for Groundskeeper and answers with JSON on stdout.
// Usage, where <source> is device (the iPhone on USB) or mac-library:
//   photos-helper list --source <source>
//   photos-helper analyze --source <source> < ids.json
//   photos-helper thumbnail --source <source> --out <dir> < ids.json
//   photos-helper hash --source <source> < ids.json
//   photos-helper delete --source <source> < requests.json
//   photos-helper access
import Foundation

let usage = """
    usage: photos-helper list --source <source>
           photos-helper analyze --source <source> < ids.json
           photos-helper thumbnail --source <source> --out <dir> < ids.json
           photos-helper hash --source <source> < ids.json
           photos-helper delete --source <source> < requests.json
           photos-helper access
    <source> is device (the iPhone on USB) or mac-library.

    """

enum Command: String {
    case list, analyze, thumbnail, hash, delete
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

/// Reads JSON from stdin, with dates in ISO 8601 as list writes them.
func readInput<T: Decodable>(_ type: T.Type) -> T {
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .iso8601
    let data = FileHandle.standardInput.readDataToEndOfFile()
    guard let value = try? decoder.decode(type, from: data) else { usageError() }
    return value
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
if args.first == "access" {
    guard args.count == 1 else { usageError() }
    emit(["access": libraryAccess()])
}
guard let command = args.first.flatMap(Command.init), let flags = parseFlags(args.dropFirst()),
    let source = flags["source"], ["device", "mac-library"].contains(source)
else { usageError() }
let allowed: Set<String> = command == .thumbnail ? ["source", "out"] : ["source"]
guard Set(flags.keys) == allowed else { usageError() }
let ids = [.analyze, .thumbnail, .hash].contains(command) ? readInput([String].self) : []
let requests = command == .delete ? readInput([DeleteRequest].self) : []
let out = flags["out"].map(outputDir)

if source == "mac-library" {
    Task {
        guard await requestLibraryAccess() else { fail(.denied) }
        switch command {
        case .list:
            let inAlbums = assetsInAlbums()
            emit(Listing(source: source, assets: libraryAssets().map { Asset($0, inAlbum: inAlbums.contains($0.localIdentifier)) }))
        case .analyze: emit(await analyze(ids, inLibrary: libraryAssets(withIDs: ids)))
        case .thumbnail: emit(await writeThumbnails(ids, inLibrary: libraryAssets(withIDs: ids), to: out!))
        case .hash: emit(await hash(ids, inLibrary: libraryAssets(withIDs: ids)))
        case .delete: emit(await delete(requests, inLibrary: libraryAssets(withIDs: requests.map(\.id))))
        }
    }
    RunLoop.main.run()
}

let catalog = DeviceCatalog { result in
    guard case .success(let files) = result else {
        if case .failure(let kind) = result { fail(kind) }
        return
    }
    switch command {
    case .list: emit(Listing(source: "device", assets: deviceAssets(files)))
    case .analyze: Task { emit(await analyze(ids, in: files)) }
    case .thumbnail: Task { emit(await writeThumbnails(ids, in: files, to: out!)) }
    case .hash: Task { emit(await hash(ids, in: files)) }
    case .delete: Task { emit(await delete(requests, in: files)) }
    }
}
catalog.start()
RunLoop.main.run()
