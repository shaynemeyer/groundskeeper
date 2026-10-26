import Foundation
import ImageCaptureCore

/// One photo or video. Holds only IDs, dates, dimensions and sizes: never GPS
/// data or file contents.
struct Asset: Encodable {
    /// Folder and file name on the phone, such as "100APPLE/IMG_0001.HEIC".
    /// ImageCaptureCore leaves originatingAssetID and fingerprint empty and
    /// gives every file the same ptpObjectHandle, so this is the only stable ID.
    let id: String
    let created: Date?
    let width: Int
    let height: Int
    let bytes: Int64
    /// Size of the Live Photo video and edit files that go with this one.
    let sidecarBytes: Int64
    let burst: String?

    init(_ file: ICCameraFile) {
        id = (file.parentFolder?.name ?? "") + "/" + (file.name ?? "")
        created = file.creationDate
        width = file.width
        height = file.height
        bytes = file.fileSize
        sidecarBytes = (file.sidecarFiles ?? []).compactMap { $0 as? ICCameraFile }.reduce(0) { $0 + $1.fileSize }
        burst = file.burstUUID
    }
}
