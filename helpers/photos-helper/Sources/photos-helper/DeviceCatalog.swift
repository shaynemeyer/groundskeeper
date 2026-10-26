import Foundation
import ImageCaptureCore

/// Opens the first iPhone on USB with ImageCaptureCore and hands its camera
/// roll to done once the catalog is complete.
@MainActor
final class DeviceCatalog: NSObject {
    /// How long to wait for a phone to show up.
    static let findTimeout: TimeInterval = 10
    /// How long to wait for the catalog, which includes time to unlock.
    static let catalogTimeout: TimeInterval = 120

    private let browser = ICDeviceBrowser()
    private let done: (Result<[ICCameraFile], ErrorKind>) -> Void
    private var camera: ICCameraDevice?

    init(done: @escaping (Result<[ICCameraFile], ErrorKind>) -> Void) {
        self.done = done
    }

    func start() {
        browser.delegate = self
        browser.browsedDeviceTypeMask = ICDeviceTypeMask(
            rawValue: ICDeviceTypeMask.camera.rawValue | ICDeviceLocationTypeMask.local.rawValue)!
        browser.start()
        after(Self.findTimeout) { if $0.camera == nil { $0.done(.failure(.noDevice)) } }
        after(Self.catalogTimeout) {
            $0.done(.failure($0.camera?.isAccessRestrictedAppleDevice == true ? .locked : .timeout))
        }
    }

    private func after(_ seconds: TimeInterval, _ body: @escaping @MainActor (DeviceCatalog) -> Void) {
        DispatchQueue.main.asyncAfter(deadline: .now() + seconds) { body(self) }
    }
}

extension DeviceCatalog: @preconcurrency ICDeviceBrowserDelegate {
    func deviceBrowser(_ browser: ICDeviceBrowser, didAdd device: ICDevice, moreComing: Bool) {
        guard camera == nil, let found = device as? ICCameraDevice else { return }
        camera = found
        found.delegate = self
        found.requestOpenSession()
    }

    func deviceBrowser(_ browser: ICDeviceBrowser, didRemove device: ICDevice, moreGoing: Bool) {}
}

/// A locked phone fails to open with -9943 and then delivers the catalog once
/// it is unlocked, so open errors are not treated as final.
extension DeviceCatalog: @preconcurrency ICCameraDeviceDelegate {
    func deviceDidBecomeReady(withCompleteContentCatalog device: ICCameraDevice) {
        done(.success((device.mediaFiles ?? []).compactMap { $0 as? ICCameraFile }))
    }

    func device(_ device: ICDevice, didOpenSessionWithError error: (any Error)?) {}
    func device(_ device: ICDevice, didCloseSessionWithError error: (any Error)?) {}
    func didRemove(_ device: ICDevice) {}
    func cameraDevice(_ camera: ICCameraDevice, didAdd items: [ICCameraItem]) {}
    func cameraDevice(_ camera: ICCameraDevice, didRemove items: [ICCameraItem]) {}
    func cameraDevice(_ camera: ICCameraDevice, didRenameItems items: [ICCameraItem]) {}
    func cameraDevice(_ camera: ICCameraDevice, didReceiveThumbnail thumbnail: CGImage?, for item: ICCameraItem, error: (any Error)?) {}
    func cameraDevice(_ camera: ICCameraDevice, didReceiveMetadata metadata: [AnyHashable: Any]?, for item: ICCameraItem, error: (any Error)?) {}
    func cameraDeviceDidChangeCapability(_ camera: ICCameraDevice) {}
    func cameraDevice(_ camera: ICCameraDevice, didReceivePTPEvent eventData: Data) {}
    func cameraDeviceDidRemoveAccessRestriction(_ device: ICDevice) {}
    func cameraDeviceDidEnableAccessRestriction(_ device: ICDevice) {}
}
