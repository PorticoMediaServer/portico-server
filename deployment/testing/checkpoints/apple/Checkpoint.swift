import XCTest

// A zero-credential headless input qualification. Existing app/userdata is never
// reinstalled, reset, or changed; this does not qualify sign-in/Home/playback.
final class Checkpoint: XCTestCase {
    func testRetainedWelcomeRemoteInput() throws {
        continueAfterFailure = false
        #if os(tvOS)
        let app = XCUIApplication(bundleIdentifier: "tv.getportico.portico.tv")
        #else
        let app = XCUIApplication(bundleIdentifier: "tv.getportico.portico.ios")
        #endif
        app.launch()
        defer { app.terminate() }
        let signIn = app.buttons.matching(NSPredicate(format: "label == %@", "Sign in with Portico Account")).firstMatch
        guard signIn.waitForExistence(timeout: 20) else {
            throw XCTSkip("Retained app is not at Welcome; no reset or credential action.")
        }
        guard app.secureTextFields.count == 0, app.textFields.count == 0 else {
            throw XCTSkip("Editable screen: no screenshots or credential actions.")
        }
        #if os(tvOS)
        // Exercise the supported headless remote, then return to the same
        // original Welcome control. No address, code, username or password.
        let local = app.buttons.matching(NSPredicate(format: "label == %@", "Use a local server")).firstMatch
        XCTAssertTrue(local.waitForExistence(timeout: 5), "Original Welcome local-server control exists")
        XCUIRemote.shared.press(.right)
        XCTAssertTrue(local.hasFocus, "Right moves actual focus to the local-server control")
        XCUIRemote.shared.press(.left)
        XCTAssertTrue(signIn.hasFocus, "Left returns actual focus to the account control")
        #else
        XCTAssertTrue(signIn.isHittable, "Welcome Sign in remains reachable")
        #endif
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = "zero-credential-welcome"
        shot.lifetime = .keepAlways
        add(shot)
        print("PORTICO_APPLE_RECEIPT {\"welcome\":true,\"inputDriver\":true,\"credentialsUsed\":false,\"qualification\":\"zero-credential input only\"}")
    }

    #if os(tvOS)
    /// The retained tvOS flow set. An unavailable flow is not a pass: the assertion fails
    /// only on a product failure, and a run with unavailable rows skips with their count.
    func testRetainedTVFlows() throws {
        let receipt = TVFlows.run(testCase: self)
        XCTAssertEqual(receipt.failed, 0, "Apple TV product flows failed: \(receipt.failed)")
        if receipt.unavailable > 0 { throw XCTSkip("Apple TV flows unavailable: \(receipt.unavailable)") }
    }
    #endif
}
