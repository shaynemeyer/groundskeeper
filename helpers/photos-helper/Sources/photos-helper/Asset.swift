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

    /// A file from the phone. A photo with its edited render is marked edited,
    /// and the render's size is counted in sidecarBytes.
    init(_ file: ICCameraFile, render: ICCameraFile? = nil) {
        id = file.assetID
        created = file.creationDate
        width = file.width
        height = file.height
        bytes = file.fileSize
        sidecarBytes = (file.sidecarFiles ?? []).compactMap { $0 as? ICCameraFile }.reduce(0) { $0 + $1.fileSize }
            + (render?.fileSize ?? 0)
        burst = file.burstUUID
        favorite = nil
        edited = render == nil ? nil : true
        inAlbum = nil
    }

    /// The original photo or video is bytes; everything else, such as the
    /// Live Photo video, edits and the edited render, is sidecarBytes.
    init(_ asset: PHAsset, inAlbum: Bool) {
        let resources = PHAssetResource.assetResources(for: asset)
        let original = originalResource(in: resources)
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

/// The original photo, video or audio file among an asset's resources. Each
/// call to assetResources(for:) returns new objects, so callers compare the
/// result by identity only against the array they passed in.
func originalResource(in resources: [PHAssetResource]) -> PHAssetResource? {
    resources.first { [.photo, .video, .audio].contains($0.type) }
}

/// The phone's camera roll as Photos shows it. iOS sends an edited photo's
/// render as a file of its own, such as IMG_E0001.JPG next to IMG_0001.HEIC.
/// A render with exactly one original in the same folder, taken at the same
/// moment, is folded into that original; any other file stays an asset.
func deviceAssets(_ files: [ICCameraFile]) -> [Asset] {
    let byKey = Dictionary(grouping: files) { $0.photoKey }
    var renders: [ObjectIdentifier: ICCameraFile] = [:]
    for file in files {
        if let key = file.originalKey, let originals = byKey[key], originals.count == 1 {
            renders[ObjectIdentifier(originals[0])] = file
        }
    }
    let folded = Set(renders.values.map(ObjectIdentifier.init))
    return files.filter { !folded.contains(ObjectIdentifier($0)) }.map {
        Asset($0, render: renders[ObjectIdentifier($0)])
    }
}

extension ICCameraFile {
    /// Folder and file name on the phone, such as "202410__/IMG_0001.HEIC".
    /// ImageCaptureCore leaves originatingAssetID and fingerprint empty and
    /// gives every file the same ptpObjectHandle, so this is the only stable ID.
    var assetID: String {
        (parentFolder?.name ?? "") + "/" + (name ?? "")
    }

    /// Folder, file name without extension, and creation date.
    var photoKey: String? {
        key(stem: ((name ?? "") as NSString).deletingPathExtension)
    }

    /// For a render such as IMG_E0001.JPG, the photoKey its original IMG_0001
    /// has. Nil for any other file.
    var originalKey: String? {
        guard let name, name.hasPrefix("IMG_E") else { return nil }
        return key(stem: "IMG_" + (String(name.dropFirst(5)) as NSString).deletingPathExtension)
    }

    private func key(stem: String) -> String? {
        guard let creationDate else { return nil }
        return "\(parentFolder?.name ?? "")/\(stem)@\(creationDate.timeIntervalSince1970)"
    }
}
