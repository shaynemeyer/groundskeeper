// photos-helper reads photos for Groundskeeper and answers with JSON on stdout.
// Usage: photos-helper list --source device
import Foundation

let usage = "usage: photos-helper list --source device\n"

let args = Array(CommandLine.arguments.dropFirst())
guard args == ["list", "--source", "device"] else {
    FileHandle.standardError.write(Data(usage.utf8))
    exit(2)
}

let catalog = DeviceCatalog { result in
    switch result {
    case .success(let files): emit(Listing(source: "device", assets: files.map(Asset.init)))
    case .failure(let kind): fail(kind)
    }
}
catalog.start()
RunLoop.main.run()
