@preconcurrency import ImageCaptureCore
import Foundation
import ImageIO
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

/// Analyzes the given assets one at a time, in the order given.
@MainActor
func analyze(_ ids: [String], in files: [ICCameraFile]) async -> Analyses {
    let byID = Dictionary(files.map { ($0.assetID, $0) }, uniquingKeysWith: { first, _ in first })
    var analyses: [Analysis] = []
    var failed: [Failure] = []
    for id in ids {
        guard let file = byID[id] else {
            failed.append(Failure(id: id, reason: .notFound))
            continue
        }
        guard let image = await thumbnail(of: file) else {
            failed.append(Failure(id: id, reason: .thumbnail))
            continue
        }
        guard let analysis = try? await analyze(image, id: id) else {
            failed.append(Failure(id: id, reason: .vision))
            continue
        }
        analyses.append(analysis)
    }
    return Analyses(source: "device", analyses: analyses, failed: failed)
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
