import Foundation
import ImageCaptureCore
import Photos

/// One photo or video. Holds only IDs, dates, dimensions and sizes: never GPS
/// data or file contents.
struct Asset: Encodable {
    let id: String
    let created: Date?
    let width: Int
    let height: Int
    let bytes: Int64
    /// Size of the Live Photo video and edit files that go with this one.
    let sidecarBytes: Int64
    let burst: String?
    /// Favorite, edited and album status come only from the Mac library: they
    /// aren't visible over USB, so they're left out on the device path.
    let favorite: Bool?
    let edited: Bool?
    let inAlbum: Bool?

    init(_ file: ICCameraFile) {
        id = file.assetID
        created = file.creationDate
        width = file.width
        height = file.height
        bytes = file.fileSize
        sidecarBytes = (file.sidecarFiles ?? []).compactMap { $0 as? ICCameraFile }.reduce(0) { $0 + $1.fileSize }
        burst = file.burstUUID
        favorite = nil
        edited = nil
        inAlbum = nil
    }

    /// The original photo or video is bytes; everything else, such as the
    /// Live Photo video, edits and the edited render, is sidecarBytes.
    init(_ asset: PHAsset, inAlbum: Bool) {
        let resources = PHAssetResource.assetResources(for: asset)
        let original = resources.first { [.photo, .video, .audio].contains($0.type) }
        id = asset.localIdentifier
        created = asset.creationDate
        width = asset.pixelWidth
        height = asset.pixelHeight
        bytes = original?.bytes ?? 0
        sidecarBytes = resources.filter { $0 !== original }.reduce(0) { $0 + $1.bytes }
        burst = asset.burstIdentifier
        favorite = asset.isFavorite
        edited = asset.hasAdjustments
        self.inAlbum = inAlbum
    }
}

extension ICCameraFile {
    /// Folder and file name on the phone, such as "202410__/IMG_0001.HEIC".
    /// ImageCaptureCore leaves originatingAssetID and fingerprint empty and
    /// gives every file the same ptpObjectHandle, so this is the only stable ID.
    var assetID: String {
        (parentFolder?.name ?? "") + "/" + (name ?? "")
    }
}
