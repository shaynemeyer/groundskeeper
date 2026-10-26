import Foundation

/// The answer to `list`: every photo and video in the source.
struct Listing: Encodable {
    let source: String
    let assets: [Asset]
}

/// An asset a command couldn't handle, with the step that failed.
struct Failure: Encodable {
    enum Reason: String, Encodable {
        case notFound = "not-found"
        case thumbnail
        case vision
        case write
        case mismatch
        case delete
        case read
    }

    let id: String
    let reason: Reason
}

/// Error kinds. Messages from Apple's frameworks are never passed through,
/// because ImageCaptureCore's contain the iPhone's name.
enum ErrorKind: String, Error, Encodable {
    case noDevice = "no-device"
    case locked
    case timeout
    /// Photos access for the Mac library wasn't granted.
    case denied
}

/// Writes value as JSON to stdout and exits 0.
func emit(_ value: some Encodable) -> Never {
    write(value)
    exit(0)
}

/// Writes {"error": kind} to stdout and exits 1.
func fail(_ kind: ErrorKind) -> Never {
    write(["error": kind])
    exit(1)
}

private func write(_ value: some Encodable) {
    let encoder = JSONEncoder()
    encoder.dateEncodingStrategy = .iso8601
    encoder.outputFormatting = .sortedKeys
    FileHandle.standardOutput.write(try! encoder.encode(value))
    FileHandle.standardOutput.write(Data("\n".utf8))
}
