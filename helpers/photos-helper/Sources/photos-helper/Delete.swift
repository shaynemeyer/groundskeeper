@preconcurrency import ImageCaptureCore
import Foundation

/// The answer to `delete`.
struct Deletion: Encodable {
    let source: String
    let deleted: [String]
    let failed: [Failure]
}

/// An asset to delete, with the size and date the caller last saw. Both must
/// still match, so a reused file name can never delete a different photo.
struct DeleteRequest: Decodable {
    let id: String
    let bytes: Int64
    let created: Date
}

/// Deletes the requested assets from the phone in one request. These deletes
/// are permanent: they skip the phone's Recently Deleted.
@MainActor
func delete(_ requests: [DeleteRequest], in files: [ICCameraFile]) async -> Deletion {
    let byID = Dictionary(files.map { ($0.assetID, $0) }, uniquingKeysWith: { first, _ in first })
    var matched: [ICCameraFile] = []
    var failed: [Failure] = []
    for request in requests {
        guard let file = byID[request.id] else {
            failed.append(Failure(id: request.id, reason: .notFound))
            continue
        }
        guard matches(file, request) else {
            failed.append(Failure(id: request.id, reason: .mismatch))
            continue
        }
        matched.append(file)
    }
    guard let camera = matched.first?.device else {
        return Deletion(source: "device", deleted: [], failed: failed)
    }
    let deleted = await deleteFiles(matched, on: camera)
    let notDeleted = matched.map(\.assetID).filter { !deleted.contains($0) }
    failed += notDeleted.map { Failure(id: $0, reason: .delete) }
    return Deletion(source: "device", deleted: matched.map(\.assetID).filter(deleted.contains), failed: failed)
}

/// Size must be equal and the date within a second, because list writes
/// dates without fractions of a second.
private func matches(_ file: ICCameraFile, _ request: DeleteRequest) -> Bool {
    guard let created = file.creationDate else { return false }
    return file.fileSize == request.bytes && abs(created.timeIntervalSince(request.created)) < 1
}

/// Returns the IDs of the files the phone reports as deleted. The blocks are
/// @Sendable because ImageCaptureCore calls them on a background queue; a
/// closure inferred as @MainActor traps there in Swift 6.
@MainActor
private func deleteFiles(_ files: [ICCameraFile], on camera: ICCameraDevice) async -> Set<String> {
    await withCheckedContinuation { done in
        camera.requestDeleteFiles(files, deleteFailed: { @Sendable _ in }) { @Sendable result, _ in
            let items = result[.successful] ?? []
            done.resume(returning: Set(items.compactMap { ($0 as? ICCameraFile)?.assetID }))
        }
    }
}
