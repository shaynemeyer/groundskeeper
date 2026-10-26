@preconcurrency import ImageCaptureCore
import CryptoKit
import Foundation

/// The answer to `thumbnail`.
struct Thumbnails: Encodable {
    let source: String
    let thumbnails: [Thumbnail]
    let failed: [Failure]
}

/// A JPEG thumbnail written for the review screen.
struct Thumbnail: Encodable {
    let id: String
    let path: String
}

/// Longest side of review thumbnails: big enough to tell near-duplicates
/// apart in Quick Look, and as fast to fetch as smaller ones.
let reviewPixels = 1024

/// Writes a thumbnail of each asset into dir, which is created 0700 if
/// missing. Files are 0600 and named by a hash of the ID, because IDs come
/// from the phone and must not be used as paths.
@MainActor
func writeThumbnails(_ ids: [String], in files: [ICCameraFile], to dir: URL) async -> Thumbnails {
    let byID = Dictionary(files.map { ($0.assetID, $0) }, uniquingKeysWith: { first, _ in first })
    var thumbnails: [Thumbnail] = []
    var failed: [Failure] = []
    for id in ids {
        guard let file = byID[id] else {
            failed.append(Failure(id: id, reason: .notFound))
            continue
        }
        guard let data = await thumbnailData(of: file, pixels: reviewPixels) else {
            failed.append(Failure(id: id, reason: .thumbnail))
            continue
        }
        let path = dir.appending(path: fileName(for: id)).path
        guard FileManager.default.createFile(atPath: path, contents: data, attributes: [.posixPermissions: 0o600]) else {
            failed.append(Failure(id: id, reason: .write))
            continue
        }
        thumbnails.append(Thumbnail(id: id, path: path))
    }
    return Thumbnails(source: "device", thumbnails: thumbnails, failed: failed)
}

/// The first 16 bytes of the ID's SHA-256, in hex, plus ".jpg".
func fileName(for id: String) -> String {
    SHA256.hash(data: Data(id.utf8)).prefix(16).map { String(format: "%02x", $0) }.joined() + ".jpg"
}

/// Fetches a JPEG thumbnail from the phone, with its longest side at most
/// pixels. Returns nil if the phone doesn't send one.
@MainActor
func thumbnailData(of file: ICCameraFile, pixels: Int) async -> Data? {
    await withCheckedContinuation { done in
        file.requestThumbnailData(options: [.imageSourceThumbnailMaxPixelSize: pixels]) { data, _ in
            done.resume(returning: data)
        }
    }
}
