@preconcurrency import ImageCaptureCore
import Foundation
import ImageIO
import Photos
import Vision

/// The answer to `analyze`.
struct Analyses: Encodable {
    let source: String
    let analyses: [Analysis]
    let failed: [Failure]
}

/// Vision results for one asset, computed on a thumbnail.
struct Analysis: Encodable {
    let id: String
    /// 768 little-endian float32 values, base64 encoded. Vision's distance
    /// between two prints is the squared Euclidean distance of these vectors.
    let featurePrint: Data
    /// Vision's overall aesthetics score, from -1 to 1.
    let aesthetics: Float
    /// True for screenshots, receipts and other non-memorable images.
    let utility: Bool
}

/// Longest side of the thumbnail Vision runs on. Larger thumbnails took as
/// long to fetch and gave almost the same scores.
let analysisPixels = 512

/// Analyzes the given assets one at a time, in the order given. find looks
/// an ID up in the source and image fetches its thumbnail.
@MainActor
func analyze<Item>(
    _ ids: [String], source: String, find: (String) -> Item?, image: (Item) async -> CGImage?
) async -> Analyses {
    var analyses: [Analysis] = []
    var failed: [Failure] = []
    for id in ids {
        guard let item = find(id) else {
            failed.append(Failure(id: id, reason: .notFound))
            continue
        }
        guard let image = await image(item) else {
            failed.append(Failure(id: id, reason: .thumbnail))
            continue
        }
        guard let analysis = try? await analyze(image, id: id) else {
            failed.append(Failure(id: id, reason: .vision))
            continue
        }
        analyses.append(analysis)
    }
    return Analyses(source: source, analyses: analyses, failed: failed)
}

/// Analyzes assets on the phone.
@MainActor
func analyze(_ ids: [String], in files: [ICCameraFile]) async -> Analyses {
    let byID = Dictionary(files.map { ($0.assetID, $0) }, uniquingKeysWith: { first, _ in first })
    return await analyze(ids, source: "device", find: { byID[$0] }, image: { await thumbnail(of: $0) })
}

/// Analyzes assets in the Mac Photos library.
@MainActor
func analyze(_ ids: [String], inLibrary assets: [String: PHAsset]) async -> Analyses {
    await analyze(ids, source: "mac-library", find: { assets[$0] }, image: { await image(of: $0, pixels: analysisPixels) })
}

private func analyze(_ image: CGImage, id: String) async throws -> Analysis {
    let print = try await GenerateImageFeaturePrintRequest().perform(on: image)
    let scores = try await CalculateImageAestheticsScoresRequest().perform(on: image)
    return Analysis(id: id, featurePrint: print.data, aesthetics: scores.overallScore, utility: scores.isUtility)
}

/// Fetches a thumbnail from the phone and decodes it.
@MainActor
private func thumbnail(of file: ICCameraFile) async -> CGImage? {
    guard let data = await thumbnailData(of: file, pixels: analysisPixels),
        let source = CGImageSourceCreateWithData(data as CFData, nil)
    else { return nil }
    return CGImageSourceCreateImageAtIndex(source, 0, nil)
}
