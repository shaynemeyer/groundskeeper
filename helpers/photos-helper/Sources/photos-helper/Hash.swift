@preconcurrency import ImageCaptureCore
import CryptoKit
import Foundation
import Photos

/// The answer to `hash`.
struct Hashes: Encodable {
    let source: String
    let hashes: [FileHash]
    let failed: [Failure]
}

/// The SHA-256 of an asset's original file, in hex. Only the hash is written,
/// never the file's contents.
struct FileHash: Encodable {
    let id: String
    let sha256: String
}

/// How much of a file on the phone is read per USB request.
let readChunk: Int64 = 4 << 20

/// Hashes each asset's original file, in the order given. find looks an ID up
/// in the source and digest streams the file through SHA-256.
@MainActor
func hash<Item>(
    _ ids: [String], source: String, find: (String) -> Item?, digest: (Item) async -> SHA256Digest?
) async -> Hashes {
    var hashes: [FileHash] = []
    var failed: [Failure] = []
    for id in ids {
        guard let item = find(id) else {
            failed.append(Failure(id: id, reason: .notFound))
            continue
        }
        guard let digest = await digest(item) else {
            failed.append(Failure(id: id, reason: .read))
            continue
        }
        hashes.append(FileHash(id: id, sha256: digest.map { String(format: "%02x", $0) }.joined()))
    }
    return Hashes(source: source, hashes: hashes, failed: failed)
}

/// Hashes files on the phone, read in chunks over USB without saving them.
@MainActor
func hash(_ ids: [String], in files: [ICCameraFile]) async -> Hashes {
    let byID = Dictionary(files.map { ($0.assetID, $0) }, uniquingKeysWith: { first, _ in first })
    return await hash(ids, source: "device", find: { byID[$0] }) { await digest(of: $0) }
}

/// Hashes the original of each Mac library asset, as stored in the library.
@MainActor
func hash(_ ids: [String], inLibrary assets: [String: PHAsset]) async -> Hashes {
    await hash(ids, source: "mac-library", find: { assets[$0] }) { asset in
        guard let original = originalResource(of: asset) else { return nil }
        return await digest(of: original)
    }
}

/// Reads the whole file from the phone. Returns nil if a read fails or comes
/// back empty before the end. The block is @Sendable because ImageCaptureCore
/// calls it on any queue.
@MainActor
private func digest(of file: ICCameraFile) async -> SHA256Digest? {
    var sha = SHA256()
    var offset: Int64 = 0
    while offset < file.fileSize {
        let length = min(readChunk, file.fileSize - offset)
        let data = await withCheckedContinuation { done in
            file.requestReadData(atOffset: off_t(offset), length: off_t(length)) { @Sendable data, _ in
                done.resume(returning: data)
            }
        }
        guard let data, !data.isEmpty else { return nil }
        sha.update(data: data)
        offset += Int64(data.count)
    }
    return sha.finalize()
}

/// Streams a library resource through SHA-256. Network access is off, so an
/// original stored only in iCloud gives nil.
private func digest(of resource: PHAssetResource) async -> SHA256Digest? {
    let options = PHAssetResourceRequestOptions()
    options.isNetworkAccessAllowed = false
    let sha = StreamingHash()
    return await withCheckedContinuation { done in
        PHAssetResourceManager.default().requestData(for: resource, options: options) { @Sendable data in
            sha.update(data)
        } completionHandler: { @Sendable error in
            done.resume(returning: error == nil ? sha.finalize() : nil)
        }
    }
}

/// SHA-256 state shared by PhotoKit's handlers, which Apple documents as
/// called on one serial queue, so they never run at the same time.
private final class StreamingHash: @unchecked Sendable {
    private var sha = SHA256()

    func update(_ data: Data) { sha.update(data: data) }
    func finalize() -> SHA256Digest { sha.finalize() }
}
