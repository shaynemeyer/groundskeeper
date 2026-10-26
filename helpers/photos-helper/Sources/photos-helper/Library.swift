import Foundation
import ImageIO
import Photos

/// Asks for Photos access. macOS grants it to the terminal app the helper runs
/// from, prompts only the first time, and denies without a prompt in hardened
/// apps such as VS Code.
func requestLibraryAccess() async -> Bool {
    await PHPhotoLibrary.requestAuthorization(for: .readWrite) == .authorized
}

/// The Photos access macOS has recorded for the app the helper runs from,
/// without prompting.
func libraryAccess() -> String {
    switch PHPhotoLibrary.authorizationStatus(for: .readWrite) {
    case .authorized: "authorized"
    case .limited: "limited"
    case .denied: "denied"
    case .restricted: "restricted"
    case .notDetermined: "not-determined"
    @unknown default: "unknown"
    }
}

/// Every photo and video in the Mac Photos library, oldest first. Every photo
/// of a burst is included, as on the device path; hidden photos are not.
func libraryAssets() -> [PHAsset] {
    let options = PHFetchOptions()
    options.sortDescriptors = [NSSortDescriptor(key: "creationDate", ascending: true)]
    options.includeAllBurstAssets = true
    let result = PHAsset.fetchAssets(with: options)
    return result.objects(at: IndexSet(integersIn: 0..<result.count))
}

/// IDs of the assets in at least one album.
func assetsInAlbums() -> Set<String> {
    var ids = Set<String>()
    let albums = PHAssetCollection.fetchAssetCollections(with: .album, subtype: .any, options: nil)
    albums.enumerateObjects { album, _, _ in
        PHAsset.fetchAssets(in: album, options: nil).enumerateObjects { asset, _, _ in
            ids.insert(asset.localIdentifier)
        }
    }
    return ids
}

/// The library assets with the given IDs, by ID. Unknown IDs are left out.
func libraryAssets(withIDs ids: [String]) -> [String: PHAsset] {
    var assets: [String: PHAsset] = [:]
    PHAsset.fetchAssets(withLocalIdentifiers: ids, options: nil).enumerateObjects { asset, _, _ in
        assets[asset.localIdentifier] = asset
    }
    return assets
}

/// Decodes the asset's current version, edits included, with its longest side
/// at most pixels. requestImage gave nil with no error for every photo when
/// run from a terminal, while the image data arrived intact, so ImageIO does
/// the scaling. Network access is off, so a photo only in iCloud gives nil.
@MainActor
func image(of asset: PHAsset, pixels: Int) async -> CGImage? {
    let options = PHImageRequestOptions()
    options.deliveryMode = .highQualityFormat
    options.isNetworkAccessAllowed = false
    let data = await withCheckedContinuation { done in
        PHImageManager.default().requestImageDataAndOrientation(for: asset, options: options) { @Sendable data, _, _, _ in
            done.resume(returning: data)
        }
    }
    guard let data, let source = CGImageSourceCreateWithData(data as CFData, nil) else { return nil }
    let thumbnail: [CFString: Any] = [
        kCGImageSourceCreateThumbnailFromImageAlways: true,
        kCGImageSourceCreateThumbnailWithTransform: true,
        kCGImageSourceThumbnailMaxPixelSize: pixels,
    ]
    return CGImageSourceCreateThumbnailAtIndex(source, 0, thumbnail as CFDictionary)
}

extension PHAssetResource {
    /// Size in bytes, from the undocumented "fileSize" key the step 8 spike
    /// found. Zero if it is missing.
    var bytes: Int64 {
        (value(forKey: "fileSize") as? NSNumber)?.int64Value ?? 0
    }
}
