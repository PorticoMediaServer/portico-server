import Foundation
import XCTest

/// Apple TV (tvOS) M0 flow source for the existing headless checkpoint target.
///
/// Driver target hook, applied in Driver.xcodeproj: a PBXFileReference, a PBXBuildFile and a
/// member of the existing PorticoTVCheckpoint target's Sources phase, beside Checkpoint.swift and
/// PhoneFlows.swift. XCTest and XCUIRemote come from that same UI-testing target; no dependency,
/// product, scheme or target was added. The optional `Debug-iOS` configuration on the target is
/// phone-only and does not affect this tvOS source, and the default `Debug` configuration and the
/// shared scheme stay exactly as they were.
///
/// The test that runs this source is tvOS-only and lives in Checkpoint.swift:
///     Checkpoint/testRetainedTVFlows
/// which asserts zero failed flows and skips when any flow is unavailable. The machine wrapper
/// selects runs by test identifier only, and this source performs no runtime activation itself.
///
/// Only the exact retained bundle is launched. Nothing is installed, reset or re-authenticated;
/// no password, address, code or account value is read. No screenshot is attached here: a
/// signed-in screen can carry account or profile content, and the existing e765 Welcome
/// attachment stays exactly as it is. Focus counts only when XCUIElement.hasFocus is true after
/// an actual remote press. Play passes only when the fixture is visible and the app's own
/// accessible playback-position control advances; an absent or coarse port is unavailable,
/// never an image change.

enum TVFlowID: String {
    case retainedWelcome = "apple-tv-retained-welcome"
    case retainedState = "apple-tv-retained-state"
    case home = "apple-tv-home"
    case focus = "apple-tv-focus"
    case browse = "apple-tv-browse"
    case detail = "apple-tv-detail"
    case play = "apple-tv-play"
    case settingsBack = "apple-tv-settings-back"
}

enum TVFlowStatus: String {
    case passed
    case failed
    case unavailable
}

struct TVFlowResult {
    let id: TVFlowID
    let status: TVFlowStatus
    let reason: String
}

struct TVFlowReceipt {
    let surface = "tvos"
    var authState = "unknown"
    private(set) var results: [TVFlowResult] = []

    var passed: Int { results.filter { $0.status == .passed }.count }
    var failed: Int { results.filter { $0.status == .failed }.count }
    var unavailable: Int { results.filter { $0.status == .unavailable }.count }

    mutating func add(_ id: TVFlowID, _ status: TVFlowStatus, _ reason: String = "") {
        results.append(TVFlowResult(id: id, status: status, reason: reason))
    }
}

#if os(tvOS)
final class TVFlows {
    static let bundleIdentifier = "tv.getportico.portico.tv"
    static let fixtureTitle = "12 Angry Men"

    private static let homeRows = ["Continue Watching", "New on", "On now", "Listen"]
    private static let settingsPages = ["Profile & account", "Playback", "Audio & subtitles", "Quality & data", "Music", "Appearance", "Language & region", "Privacy", "This device", "Account", "Server"]
    private static let maximumPresses = 6
    private static let maximumMenus = 4

    private enum FocusTarget {
        case movies, fixture, play, settings

        func matches(_ label: String) -> Bool {
            switch self {
            case .movies:
                return label == "Movies" || label.hasPrefix("Movies,")
            case .fixture:
                return label.contains(TVFlows.fixtureTitle)
            case .play:
                return label == "Play" || label == "Resume" || label.hasPrefix("Play,") || label.hasPrefix("Resume,")
            case .settings:
                return label == "Settings" || label.hasPrefix("Settings,")
            }
        }
    }

    private enum ProgressKind {
        case clock
        case fraction
    }

    private struct ProgressPort {
        let kind: ProgressKind
        let value: Double
    }

    /// Runs the retained tvOS flow set and prints one bounded receipt line.
    @discardableResult
    static func run(testCase: XCTestCase) -> TVFlowReceipt {
        var receipt = TVFlowReceipt()
        let app = XCUIApplication(bundleIdentifier: bundleIdentifier)
        app.launch()
        defer { app.terminate() }

        guard !hasEditableControls(app) else {
            receipt.authState = "editable"
            receipt.add(.retainedState, .unavailable, "editable-screen")
            emit(receipt)
            return receipt
        }

        switch retainedState(app) {
        case "retained-welcome":
            receipt.authState = "retained-welcome"
            receipt.add(.retainedWelcome, .unavailable, "auth-unavailable")
            emit(receipt)
            return receipt
        case "signed-in":
            receipt.authState = "signed-in"
        default:
            receipt.authState = "unrecognized"
            receipt.add(.retainedState, .unavailable, "retained-state-unrecognized")
            emit(receipt)
            return receipt
        }

        receipt.add(.home, .passed)

        let focus = focusWalk(app)
        receipt.add(.focus, focus.status, focus.reason)

        if focus.status == .passed {
            if openMovies(app) {
                receipt.add(.browse, .passed)
                if openDetail(app) {
                    receipt.add(.detail, .passed)
                    let play = playFlow(app)
                    receipt.add(.play, play.status, play.reason)
                } else {
                    receipt.add(.detail, .failed, "detail-not-opened")
                    receipt.add(.play, .unavailable, "detail-unavailable")
                }
            } else {
                receipt.add(.browse, .failed, "movies-not-opened")
                receipt.add(.detail, .unavailable, "browse-unavailable")
                receipt.add(.play, .unavailable, "browse-unavailable")
            }
        } else {
            receipt.add(.browse, .unavailable, "focus-unproven")
            receipt.add(.detail, .unavailable, "focus-unproven")
            receipt.add(.play, .unavailable, "focus-unproven")
        }

        let settings = settingsBack(app)
        receipt.add(.settingsBack, settings.status, settings.reason)
        emit(receipt)
        return receipt
    }

    // MARK: - Retained state

    private static func retainedState(_ app: XCUIApplication, timeout: TimeInterval = 15) -> String {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if welcomeVisible(app) { return "retained-welcome" }
            if homeVisible(app) { return "signed-in" }
            Thread.sleep(forTimeInterval: 0.4)
        }
        if welcomeVisible(app) { return "retained-welcome" }
        if homeVisible(app) { return "signed-in" }
        return "unrecognized"
    }

    private static func welcomeVisible(_ app: XCUIApplication) -> Bool {
        labelExists(app, "Sign in with Portico Account") || labelExists(app, "Use a local server")
    }

    private static func homeVisible(_ app: XCUIApplication) -> Bool {
        homeRows.contains { labelExists(app, $0) }
    }

    private static func hasEditableControls(_ app: XCUIApplication) -> Bool {
        app.secureTextFields.count > 0 || app.textFields.count > 0
    }

    /// Stage evidence needs real on-screen geometry, not just a retained route's hidden label.
    /// A few matches are checked because a hidden duplicate can precede the visible one in the
    /// tree. This never activates anything; the D-pad focus path stays the only input.
    private static func onScreen(_ element: XCUIElement) -> Bool {
        guard element.exists, element.isHittable else { return false }
        let frame = element.frame
        return frame.width > 1 && frame.height > 1
    }

    private static func anyOnScreen(_ query: XCUIElementQuery, limit: Int = 4) -> Bool {
        let count = min(query.count, limit)
        guard count > 0 else { return false }
        for index in 0..<count {
            if onScreen(query.element(boundBy: index)) { return true }
        }
        return false
    }

    private static func labelExists(_ app: XCUIApplication, _ label: String) -> Bool {
        let predicate = NSPredicate(format: "label == %@", label)
        return anyOnScreen(app.staticTexts.matching(predicate)) || anyOnScreen(app.buttons.matching(predicate))
    }

    private static func labelVisible(_ app: XCUIApplication, contains text: String) -> Bool {
        let predicate = NSPredicate(format: "label CONTAINS %@", text)
        return anyOnScreen(app.staticTexts.matching(predicate)) || anyOnScreen(app.buttons.matching(predicate))
    }

    // MARK: - Actual tvOS focus

    private static func focusedElement(_ app: XCUIApplication) -> XCUIElement? {
        // Home cards, rail items, Play and Settings are buttons; keeping the scan to that
        // collection keeps every bounded remote step cheap. A non-button focusable (the
        // playback timeline) is only relevant after Play, which targets a button.
        let query = app.buttons
        let bound = min(query.count, 80)
        guard bound > 0 else { return nil }
        for index in 0..<bound {
            let element = query.element(boundBy: index)
            if element.exists && element.hasFocus { return element }
        }
        return nil
    }

    /// Broad scan used only to start the focus walk and to classify a failed target search.
    /// Hot paths use the button-only scan above.
    private static func anyFocusedElement(_ app: XCUIApplication) -> XCUIElement? {
        let query = app.descendants(matching: .any)
        let bound = min(query.count, 120)
        guard bound > 0 else { return nil }
        for index in 0..<bound {
            let element = query.element(boundBy: index)
            if element.exists && element.hasFocus { return element }
        }
        return nil
    }

    private static func identity(_ element: XCUIElement) -> String {
        "\(element.elementType.rawValue)|\(element.identifier)|\(element.label)"
    }

    private static func waitForFocus(_ app: XCUIApplication, differentFrom previous: String, timeout: TimeInterval) -> XCUIElement? {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if let element = focusedElement(app), identity(element) != previous { return element }
            Thread.sleep(forTimeInterval: 0.2)
        }
        // A non-button focusable (for example the playback timeline) can be the first or last
        // stop of a walk. One broad scan per settled press keeps that honest without making the
        // hot path expensive.
        if let element = anyFocusedElement(app), identity(element) != previous { return element }
        return nil
    }

    private static func walk(_ app: XCUIApplication, from startKey: String, button: XCUIRemote.Button) -> (key: String, seen: Set<String>, moves: Int) {
        var key = startKey
        var seen: Set<String> = [startKey]
        var moves = 0
        for _ in 0..<maximumPresses {
            XCUIRemote.shared.press(button)
            guard let next = waitForFocus(app, differentFrom: key, timeout: 1.5) else { break }
            key = identity(next)
            seen.insert(key)
            moves += 1
        }
        return (key, seen, moves)
    }

    private static func returnTo(_ app: XCUIApplication, from startKey: String, to target: String, button: XCUIRemote.Button, moves: Int) -> Bool {
        var key = startKey
        for _ in 0..<moves {
            XCUIRemote.shared.press(button)
            guard let previous = waitForFocus(app, differentFrom: key, timeout: 1.5) else { return false }
            key = identity(previous)
        }
        return key == target
    }

    private static func focusWalk(_ app: XCUIApplication) -> (status: TVFlowStatus, reason: String) {
        guard let start = anyFocusedElement(app) else { return (.unavailable, "focus-state-not-exposed") }
        let startKey = identity(start)

        let right = walk(app, from: startKey, button: .right)
        if right.moves >= 3 && right.seen.count >= 3 {
            guard returnTo(app, from: right.key, to: startKey, button: .left, moves: right.moves) else {
                return (.failed, "focus-return-mismatch")
            }
            return (.passed, "")
        }
        guard returnTo(app, from: right.key, to: startKey, button: .left, moves: right.moves) else {
            return (.failed, "focus-return-mismatch")
        }

        let left = walk(app, from: startKey, button: .left)
        guard left.moves >= 3 && left.seen.count >= 3 else {
            return (.failed, "focus-did-not-move")
        }
        guard returnTo(app, from: left.key, to: startKey, button: .right, moves: left.moves) else {
            return (.failed, "focus-return-mismatch")
        }
        return (.passed, "")
    }

    private static func focusAndSelect(_ app: XCUIApplication, target: FocusTarget, directions: [XCUIRemote.Button]) -> Bool {
        if let focused = focusedElement(app), target.matches(focused.label) {
            XCUIRemote.shared.press(.select)
            return true
        }
        for button in directions {
            for _ in 0..<maximumPresses {
                XCUIRemote.shared.press(button)
                Thread.sleep(forTimeInterval: 0.15)
                if let focused = focusedElement(app), target.matches(focused.label) {
                    XCUIRemote.shared.press(.select)
                    return true
                }
            }
        }
        return false
    }

    // MARK: - Bounded flows

    private static func openMovies(_ app: XCUIApplication) -> Bool {
        guard focusAndSelect(app, target: .movies, directions: [.left, .down, .up, .right]) else { return false }
        let deadline = Date().addingTimeInterval(15)
        while Date() < deadline {
            if labelVisible(app, contains: fixtureTitle) && !homeVisible(app) { return true }
            Thread.sleep(forTimeInterval: 0.3)
        }
        return false
    }

    private static func openDetail(_ app: XCUIApplication) -> Bool {
        guard focusAndSelect(app, target: .fixture, directions: [.right, .down, .left, .up]) else { return false }
        let deadline = Date().addingTimeInterval(15)
        while Date() < deadline {
            if labelExists(app, "Play") || labelExists(app, "Resume") {
                return labelVisible(app, contains: fixtureTitle)
            }
            Thread.sleep(forTimeInterval: 0.3)
        }
        return false
    }

    private static func playFlow(_ app: XCUIApplication) -> (status: TVFlowStatus, reason: String) {
        guard focusAndSelect(app, target: .play, directions: [.down, .right, .left, .up]) else {
            return anyFocusedElement(app) == nil ? (.unavailable, "focus-state-not-exposed") : (.failed, "play-control-not-focusable")
        }

        var first: ProgressPort?
        let deadline = Date().addingTimeInterval(12)
        while Date() < deadline {
            first = progressPort(app)
            if first != nil { break }
            Thread.sleep(forTimeInterval: 0.3)
        }
        guard let first = first else { return (.unavailable, "progress-port-unavailable") }
        guard first.kind == .clock else { return (.unavailable, "progress-port-units-unestablished") }
        guard labelVisible(app, contains: fixtureTitle) else { return (.unavailable, "target-unproven") }

        Thread.sleep(forTimeInterval: 4.0)
        guard let later = progressPort(app) else { return (.unavailable, "progress-port-unavailable") }
        guard later.kind == first.kind else { return (.unavailable, "progress-port-kind-changed") }
        guard labelVisible(app, contains: fixtureTitle) else { return (.unavailable, "target-unproven-at-progress") }
        guard later.value > first.value + 1.0 else { return (.failed, "progress-not-advancing") }
        return (.passed, "")
    }

    private static func settingsBack(_ app: XCUIApplication) -> (status: TVFlowStatus, reason: String) {
        for _ in 0..<maximumMenus where !homeVisible(app) {
            XCUIRemote.shared.press(.menu)
            Thread.sleep(forTimeInterval: 0.5)
        }
        guard homeVisible(app) else { return (.failed, "home-not-restored") }
        guard focusAndSelect(app, target: .settings, directions: [.left, .down, .up, .right]) else {
            return anyFocusedElement(app) == nil ? (.unavailable, "focus-state-not-exposed") : (.failed, "settings-not-opened")
        }
        let deadline = Date().addingTimeInterval(8)
        var settingsDrawn = false
        while Date() < deadline {
            if labelExists(app, "Settings") && settingsPages.contains(where: { labelExists(app, $0) }) {
                settingsDrawn = true
                break
            }
            Thread.sleep(forTimeInterval: 0.3)
        }
        guard settingsDrawn else { return (.failed, "settings-content-missing") }

        XCUIRemote.shared.press(.menu)
        let backDeadline = Date().addingTimeInterval(8)
        while Date() < backDeadline {
            if homeVisible(app) { return (.passed, "") }
            Thread.sleep(forTimeInterval: 0.3)
        }
        return (.failed, "settings-back-not-returned")
    }

    // MARK: - Playback progress port

    private static func progressPort(_ app: XCUIApplication) -> ProgressPort? {
        // The player's Timeline draws a focusable adjustable element with the app's own
        // accessibility value ("0:03"); it can surface as other rather than slider, so query
        // by its fixed label. Its value is state the app publishes, not a screenshot.
        let position = app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Playback position")).firstMatch
        guard position.exists else { return nil }
        return parsedProgress(position.value)
    }

    private static func parsedProgress(_ value: Any?) -> ProgressPort? {
        if let text = value as? String {
            if let seconds = timecode(text) { return ProgressPort(kind: .clock, value: seconds) }
            if let number = Double(text) { return ProgressPort(kind: .fraction, value: number) }
            return nil
        }
        if let number = value as? NSNumber { return ProgressPort(kind: .fraction, value: number.doubleValue) }
        return nil
    }

    private static func timecode(_ text: String) -> Double? {
        let parts = text.trimmingCharacters(in: .whitespaces).split(separator: ":")
        guard parts.count == 2 || parts.count == 3 else { return nil }
        var total = 0.0
        for part in parts {
            guard let value = Double(part), value >= 0 else { return nil }
            total = total * 60 + value
        }
        return total
    }

    // MARK: - Receipt

    private static func emit(_ receipt: TVFlowReceipt) {
        let payload: [String: Any] = [
            "surface": receipt.surface,
            "authState": receipt.authState,
            "passed": receipt.passed,
            "failed": receipt.failed,
            "unavailable": receipt.unavailable,
            "flows": receipt.results.map { ["id": $0.id.rawValue, "status": $0.status.rawValue, "reason": $0.reason] }
        ]
        guard let data = try? JSONSerialization.data(withJSONObject: payload, options: [.sortedKeys]),
              let line = String(data: data, encoding: .utf8) else { return }
        print("PORTICO_APPLE_TV_RECEIPT \(line)")
    }
}
#else
final class TVFlows {
    static func run(testCase: XCTestCase) -> TVFlowReceipt {
        var receipt = TVFlowReceipt()
        receipt.authState = "unsupported"
        receipt.add(.retainedState, .unavailable, "tvos-required")
        return receipt
    }
}
#endif
