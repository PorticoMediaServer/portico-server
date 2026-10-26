import XCTest

// Bounded retained-family phone check for the Mac simulator, source only.
//
// This file adds no target, product, scheme, dependency or wrapper change. It is compiled into
// the existing `PorticoTVCheckpoint` UI-test target through the hook applied in
// Driver.xcodeproj: a PBXFileReference, a PBXBuildFile and a member of that target's Sources
// phase, beside Checkpoint.swift and TVFlows.swift.
//
// The same target carries one additional **optional** configuration, `Debug-iOS`, for the phone
// simulator: `SDKROOT=iphonesimulator`, `SUPPORTED_PLATFORMS=iphonesimulator`,
// `TARGETED_DEVICE_FAMILY=1`, `IPHONEOS_DEPLOYMENT_TARGET=18.0`. The default configuration stays
// `Debug` (tvOS), so the shared `PorticoTVCheckpoint` scheme is unchanged and no new target,
// scheme or product exists. iPad is not covered by this file or that configuration.
//
// The machine wrapper chooses a run by test identifier only: `PhoneFlows/testRetainedPhoneCore`
// for the phone, `Checkpoint/testRetainedWelcomeRemoteInput` for the zero-credential Welcome
// input check, and on tvOS `Checkpoint/testRetainedTVFlows`. Selecting the iPhone configuration
// is the wrapper's business; this source assumes no runtime activation of its own.
//
// The check drives the retained, already signed-in app: launch, Home, Libraries,
// Movies, the fixture detail, Settings and its own Back. It never installs,
// resets or re-authenticates, and it reads no file: a Welcome, credential or
// expired-session screen makes every remaining flow `unavailable` with a closed
// code, and the test skips. That recheck also runs when a Home wait times out, so
// a screen that turns into an ended session late is a gap, not a product failure.
//
// Stage evidence must be drawn: a retained route can exist in the tree without
// being on screen, so Home rows, library entries and settings pages are accepted
// only with visible geometry. Playback is reported `unavailable` rather than
// passed, because XCUI exposes no player position for this app and a tap or a
// drawn video frame is not progress.
//
// The receipt is one line of closed flow ids, statuses, reason codes and counts.
// It carries no label, title, path or identity, and screenshots are withheld
// whenever an editable control is on screen.

#if os(iOS)
final class PhoneFlows: XCTestCase {
    private enum Flow: String, CaseIterable {
        case launch = "ios-launch-restore"
        case home = "ios-home-rows"
        case libraries = "ios-libraries"
        case movies = "ios-movies"
        case detail = "ios-detail"
        case playback = "ios-playback"
        case settings = "ios-settings"
        case back = "ios-settings-back"
    }

    private enum Status: String {
        case pass, fail, unavailable
    }

    private struct Row {
        let flow: Flow
        let status: Status
        let code: String
    }

    /// Drawn client settings pages. Two of them is a real settings list.
    private static let clientPages = ["Profile & account", "Playback", "Audio & subtitles", "Quality & data",
                                      "Appearance", "Language & region", "Privacy", "This device"]
    /// Screens that mean this run must not touch the app further.
    private static let blockedLabels = ["Sign in with Portico Account", "Use a local server", "Server address",
                                        "Username", "Password", "Sign in", "Sign in again", "Session expired",
                                        "Your session ended"]
    private static let libraryEntries = ["Movies", "Shows", "Music", "Books", "Comics"]
    private static let homeRows = ".*(Continue Watching|New on|On now|Listen).*"
    private static let fixture = "12 Angry Men"
    private static let librariesTab = "phone-tab-libraries"
    private static let homeTab = "phone-tab-home"

    private var results: [Row] = []
    private var withheld = 0

    func testRetainedPhoneCore() throws {
        let app = XCUIApplication(bundleIdentifier: "tv.getportico.portico.ios")
        app.launch()
        defer { app.terminate() }

        if let blocked = blockedScreen(app) {
            for flow in Flow.allCases {
                record(flow, .unavailable, blocked)
            }
            try finish(skipping: blocked)
            return
        }

        // Launch: the retained family must draw real Home rows on screen.
        if waitHomeRows(app, timeout: 25) {
            record(.launch, .pass, "home-rows")
        } else if let blocked = blockedScreen(app) {
            record(.launch, .unavailable, blocked)
            skipRest(.launch, blocked)
            try finish(skipping: blocked)
            return
        } else {
            record(.launch, .fail, "home-rows-missing")
            skipRest(.launch, "launch-failed")
            try finish()
            return
        }

        if waitHomeRows(app, timeout: 15) {
            record(.home, .pass, "home-rows")
            capture(app, .home)
        } else if let blocked = blockedScreen(app) {
            record(.home, .unavailable, blocked)
            skipRest(.home, blocked)
            try finish(skipping: blocked)
            return
        } else {
            record(.home, .fail, "home-rows-missing")
        }

        // Libraries: the tab opens the remembered library or the list of them.
        var libraryOpen = false
        if let tab = waitElement(app, identifier: Self.librariesTab, timeout: 15) {
            tab.tap()
            libraryOpen = waitAny(app, labels: Self.libraryEntries, timeout: 20)
        }
        if libraryOpen {
            record(.libraries, .pass, "library-entry")
            capture(app, .libraries)
        } else {
            record(.libraries, .fail, "library-entry-missing")
        }

        // Movies: the fixture is on screen inside a remembered library, otherwise
        // the list's own Movies card opens it. Neither path is assumed, and the
        // card is only activated once it is visible and hittable.
        var fixtureCard: XCUIElement?
        if libraryOpen {
            fixtureCard = waitFixture(app, timeout: 8, scrolls: 1)
            if fixtureCard == nil, librariesBack(app, timeout: 8),
               let card = waitHittable(app, label: "Movies", timeout: 12) {
                card.tap()
                fixtureCard = waitFixture(app, timeout: 20, scrolls: 3)
            }
        }
        if let card = fixtureCard {
            record(.movies, .pass, "fixture-title")
            capture(app, .movies)

            // Detail: the primary control must be drawn. It is not pressed, so no
            // player state and no progress claim is invented here.
            if !card.isHittable {
                record(.detail, .fail, "card-not-hittable")
            } else {
                card.tap()
                let play = waitHittable(app, label: "Play", timeout: 15)
                let resume = play == nil ? waitHittable(app, label: "Resume", timeout: 5) : nil
                if play != nil || resume != nil {
                    record(.detail, .pass, "primary-control")
                    capture(app, .detail)
                } else {
                    record(.detail, .fail, "primary-control-missing")
                }
            }
        } else if libraryOpen {
            record(.movies, .fail, "fixture-title-missing")
            record(.detail, .unavailable, "dependency-failed")
        } else {
            record(.movies, .unavailable, "dependency-failed")
            record(.detail, .unavailable, "dependency-failed")
        }

        // XCUI exposes no player position here, so playback is an honest gap, not
        // a tap-based pass.
        record(.playback, .unavailable, "progress-unobservable")

        // Settings from Home, then its own Done.
        _ = tapLabel(app, "Back", timeout: 8)
        if let home = waitElement(app, identifier: Self.homeTab, timeout: 10) {
            home.tap()
        }
        let homeReady = waitHomeRows(app, timeout: 15)
        if homeReady {
            if tapLabel(app, "Settings", timeout: 15) {
                if waitSettings(app, timeout: 20) {
                    record(.settings, .pass, "client-pages")
                    capture(app, .settings)
                    if tapLabel(app, "Done", timeout: 10) {
                        if waitHomeRows(app, timeout: 15) {
                            record(.back, .pass, "settings-closed")
                        } else if let blocked = blockedScreen(app) {
                            record(.back, .unavailable, blocked)
                        } else {
                            record(.back, .fail, "home-rows-missing")
                        }
                    } else {
                        record(.back, .fail, "settings-done-missing")
                    }
                } else {
                    record(.settings, .fail, "settings-pages-missing")
                    record(.back, .unavailable, "dependency-failed")
                }
            } else {
                record(.settings, .fail, "settings-entry-missing")
                record(.back, .unavailable, "dependency-failed")
            }
        } else {
            let code = blockedScreen(app) ?? "dependency-failed"
            record(.settings, .unavailable, code)
            record(.back, .unavailable, code)
        }

        try finish()
    }

    // MARK: - Bounded drivers

    private func record(_ flow: Flow, _ status: Status, _ code: String) {
        results.append(Row(flow: flow, status: status, code: code))
    }

    /// Record every flow this run has not reached yet as unavailable.
    private func skipRest(_ flow: Flow, _ code: String) {
        let recorded = Set(results.map { $0.flow })
        for remaining in Flow.allCases where !recorded.contains(remaining) && remaining != flow {
            record(remaining, .unavailable, code)
        }
    }

    /// Stage evidence has to be drawn, not merely present: a hidden retained route
    /// can sit in the tree with a stale label and window-sized geometry. Hit
    /// testing is required, as in the TV helper; window containment alone never
    /// qualifies.
    private func drawn(_ app: XCUIApplication, _ element: XCUIElement) -> Bool {
        guard element.isHittable else { return false }
        let frame = element.frame
        guard !frame.isEmpty, !frame.isNull else { return false }
        let window = app.windows.firstMatch.frame
        guard !window.isEmpty, !window.isNull, window.intersects(frame) else { return false }
        return true
    }

    private func anyDrawn(_ app: XCUIApplication, _ query: XCUIElementQuery, limit: Int = 8) -> Bool {
        let count = min(query.count, limit)
        guard count > 0 else { return false }
        for index in 0..<count where drawn(app, query.element(boundBy: index)) {
            return true
        }
        return false
    }

    private func labelQuery(_ app: XCUIApplication, _ label: String) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", label))
    }

    private func waitElement(_ app: XCUIApplication, identifier: String, timeout: TimeInterval) -> XCUIElement? {
        let element = app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier == %@", identifier)).firstMatch
        guard element.waitForExistence(timeout: timeout), element.isHittable else { return nil }
        return element
    }

    private func waitHittable(_ app: XCUIApplication, label: String, timeout: TimeInterval) -> XCUIElement? {
        let button = app.buttons.matching(NSPredicate(format: "label == %@", label)).firstMatch
        if button.waitForExistence(timeout: timeout), button.isHittable {
            return button
        }
        let any = labelQuery(app, label).firstMatch
        if any.waitForExistence(timeout: 2), any.isHittable {
            return any
        }
        return nil
    }

    private func tapLabel(_ app: XCUIApplication, _ label: String, timeout: TimeInterval) -> Bool {
        guard let element = waitHittable(app, label: label, timeout: timeout) else { return false }
        element.tap()
        return true
    }

    /// The library frame's own "Libraries" back control, never the tab with the same label.
    private func librariesBack(_ app: XCUIApplication, timeout: TimeInterval) -> Bool {
        let element = labelQuery(app, "Libraries")
            .matching(NSPredicate(format: "identifier != %@", Self.librariesTab)).firstMatch
        guard element.waitForExistence(timeout: timeout), element.isHittable else { return false }
        element.tap()
        return true
    }

    private func waitAny(_ app: XCUIApplication, labels: [String], timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if labels.contains(where: { anyDrawn(app, labelQuery(app, $0)) }) { return true }
            Thread.sleep(forTimeInterval: 0.5)
        } while Date() < deadline
        return false
    }

    private func waitHomeRows(_ app: XCUIApplication, timeout: TimeInterval) -> Bool {
        let rows = app.descendants(matching: .any)
            .matching(NSPredicate(format: "label MATCHES[c] %@", Self.homeRows))
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if anyDrawn(app, rows) { return true }
            Thread.sleep(forTimeInterval: 0.5)
        } while Date() < deadline
        return false
    }

    private func waitFixture(_ app: XCUIApplication, timeout: TimeInterval, scrolls: Int) -> XCUIElement? {
        let predicate = NSPredicate(format: "label BEGINSWITH %@", Self.fixture)
        let any = app.descendants(matching: .any).matching(predicate).firstMatch
        let card = { () -> XCUIElement in
            let button = app.buttons.matching(predicate).firstMatch
            return button.exists && button.isHittable ? button : any
        }
        if any.waitForExistence(timeout: timeout) {
            return card()
        }
        // A grid that starts above the fold: bounded scrolls only.
        for _ in 0..<scrolls {
            app.swipeUp()
            if any.waitForExistence(timeout: 5) {
                return card()
            }
        }
        return nil
    }

    private func settingsPageCount(_ app: XCUIApplication) -> Int {
        Self.clientPages.reduce(0) { count, page in
            count + (anyDrawn(app, labelQuery(app, page), limit: 1) ? 1 : 0)
        }
    }

    private func waitSettings(_ app: XCUIApplication, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if settingsPageCount(app) >= 2 { return true }
            Thread.sleep(forTimeInterval: 0.5)
        } while Date() < deadline
        return false
    }

    /// A welcome, credential or expired-session screen. Nothing is typed and no
    /// file is read; the run reports it and stops.
    private func blockedScreen(_ app: XCUIApplication) -> String? {
        if app.secureTextFields.count > 0 || app.textFields.count > 0 {
            return "credential-screen"
        }
        for label in Self.blockedLabels {
            if app.buttons.matching(NSPredicate(format: "label == %@", label)).firstMatch.exists {
                return "auth-required"
            }
        }
        return nil
    }

    /// Screenshots exist only where nothing editable is drawn.
    private func capture(_ app: XCUIApplication, _ flow: Flow) {
        guard app.textFields.count == 0, app.secureTextFields.count == 0 else {
            withheld += 1
            return
        }
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = "phone-" + flow.rawValue
        shot.lifetime = .keepAlways
        add(shot)
    }

    // MARK: - Receipt

    private func receipt() -> String {
        let rows = results.map { ["id": $0.flow.rawValue, "status": $0.status.rawValue, "reason": $0.code] }
        let payload: [String: Any] = ["surface": "ios-phone", "rows": rows, "flows": results.count,
                                      "passed": results.filter { $0.status == .pass }.count,
                                      "failed": results.filter { $0.status == .fail }.count,
                                      "unavailable": results.filter { $0.status == .unavailable }.count,
                                      "credentialsUsed": false, "reset": false,
                                      "screenshotsWithheld": withheld]
        guard let data = try? JSONSerialization.data(withJSONObject: payload, options: [.sortedKeys]),
              let text = String(data: data, encoding: .utf8) else {
            return "PORTICO_APPLE_PHONE_RECEIPT {}"
        }
        return "PORTICO_APPLE_PHONE_RECEIPT " + text
    }

    private func finish(skipping reason: String? = nil) throws {
        let line = receipt()
        print(line)
        if let reason = reason {
            throw XCTSkip("PORTICO_APPLE_PHONE_UNAVAILABLE " + reason)
        }
        XCTAssertEqual(results.filter { $0.status == .fail }.count, 0, line)
    }
}
#endif
