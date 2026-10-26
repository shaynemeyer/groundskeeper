@preconcurrency import ImageCaptureCore
import CryptoKit
import Foundation
import ImageIO
import Photos
import UniformTypeIdentifiers

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
/// from the source and must not be used as paths. find looks an ID up and
/// jpeg fetches its thumbnail.
@MainActor
func writeThumbnails<Item>(
    _ ids: [String], source: String, to dir: URL, find: (String) -> Item?, jpeg: (Item) async -> Data?
) async -> Thumbnails {
    var thumbnails: [Thumbnail] = []
    var failed: [Failure] = []
    for id in ids {
        guard let item = find(id) else {
            failed.append(Failure(id: id, reason: .notFound))
            continue
        }
        guard let data = await jpeg(item) else {
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
    return Thumbnails(source: source, thumbnails: thumbnails, failed: failed)
}

/// Writes the phone's own JPEG thumbnails, without re-encoding.
@MainActor
func writeThumbnails(_ ids: [String], in files: [ICCameraFile], to dir: URL) async -> Thumbnails {
    let byID = Dictionary(files.map { ($0.assetID, $0) }, uniquingKeysWith: { first, _ in first })
    return await writeThumbnails(ids, source: "device", to: dir, find: { byID[$0] }) {
        await thumbnailData(of: $0, pixels: reviewPixels)
    }
}

/// Writes thumbnails of Mac library assets, encoded from pixels alone, so no
/// metadata such as GPS is copied.
@MainActor
func writeThumbnails(_ ids: [String], inLibrary assets: [String: PHAsset], to dir: URL) async -> Thumbnails {
    await writeThumbnails(ids, source: "mac-library", to: dir, find: { assets[$0] }) { asset in
        guard let image = await image(of: asset, pixels: reviewPixels) else { return nil }
        return jpeg(image)
    }
}

/// Encodes image as a JPEG with no metadata.
func jpeg(_ image: CGImage) -> Data? {
    let data = NSMutableData()
    guard let destination = CGImageDestinationCreateWithData(data, UTType.jpeg.identifier as CFString, 1, nil) else {
        return nil
    }
    CGImageDestinationAddImage(destination, image, [kCGImageDestinationLossyCompressionQuality: 0.8] as CFDictionary)
    return CGImageDestinationFinalize(destination) ? data as Data : nil
}

/// The first 16 bytes of the ID's SHA-256, in hex, plus ".jpg".
func fileName(for id: String) -> String {
    SHA256.hash(data: Data(id.utf8)).prefix(16).map { String(format: "%02x", $0) }.joined() + ".jpg"
}

/// Fetches a JPEG thumbnail from the phone, with its longest side at most
/// pixels. Returns nil if the phone doesn't send one. The block is @Sendable
/// because ImageCaptureCore may call it on any queue.
@MainActor
func thumbnailData(of file: ICCameraFile, pixels: Int) async -> Data? {
    await withCheckedContinuation { done in
        file.requestThumbnailData(options: [.imageSourceThumbnailMaxPixelSize: pixels]) { @Sendable data, _ in
            done.resume(returning: data)
        }
    }
}
