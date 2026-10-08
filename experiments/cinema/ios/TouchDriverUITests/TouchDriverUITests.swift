import XCTest

/// Development aid, not shipped: drives the app in the iOS Simulator from a command file so every
/// screen can be walked and screenshotted headlessly.
///
/// Commands, one per line: tap <x> <y> (points), swipe up|down|left|right, open <url> (accepts the
/// system "Open in" prompt), type <text>, orient landscape|portrait, wait <seconds>, launch, stop.
final class TouchDriverUITests: XCTestCase {
  func testTouchDriver() throws {
    let env = ProcessInfo.processInfo.environment
    let path = env["TOUCH_DRIVER_FILE"] ?? "/tmp/cinema-touch/cmd.txt"
    let ack = path + ".ack"
    let app = XCUIApplication(bundleIdentifier: "tv.getportico.experiment.cinema.ios")
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    var seen = ""
    let deadline = Date().addingTimeInterval(TimeInterval(env["TOUCH_DRIVER_MINUTES"].flatMap(Double.init) ?? 240) * 60)
    while Date() < deadline {
      guard let text = try? String(contentsOfFile: path, encoding: .utf8), text != seen, !text.isEmpty else {
        Thread.sleep(forTimeInterval: 0.25)
        continue
      }
      seen = text
      for raw in text.split(separator: "\n") {
        let parts = raw.split(separator: " ", maxSplits: 1).map(String.init)
        guard let cmd = parts.first else { continue }
        let arg = parts.count > 1 ? parts[1] : ""
        let origin = app.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0))
        switch cmd {
        case "tap":
          let xy = arg.split(separator: " ").compactMap { Double($0) }
          if xy.count == 2 { origin.withOffset(CGVector(dx: xy[0], dy: xy[1])).tap() }
        case "longpress":
          let xy = arg.split(separator: " ").compactMap { Double($0) }
          if xy.count == 2 { origin.withOffset(CGVector(dx: xy[0], dy: xy[1])).press(forDuration: 1.0) }
        case "swipe":
          switch arg {
          case "up": app.swipeUp()
          case "down": app.swipeDown()
          case "left": app.swipeLeft()
          case "right": app.swipeRight()
          default: break
          }
        case "scroll":
          let dy = Double(arg) ?? 300
          let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.6))
          start.press(forDuration: 0.05, thenDragTo: start.withOffset(CGVector(dx: 0, dy: -dy)))
        case "open":
          if #available(iOS 16.4, *), let url = URL(string: arg) {
            app.open(url)
            let open = springboard.buttons["Open"]
            if open.waitForExistence(timeout: 2) { open.tap() }
          }
        case "orient":
          XCUIDevice.shared.orientation = arg == "landscape" ? .landscapeLeft : .portrait
        case "type": app.typeText(arg)
        case "launch": app.activate()
        case "wait": Thread.sleep(forTimeInterval: Double(arg) ?? 1)
        case "stop": return
        default: break
        }
        Thread.sleep(forTimeInterval: 0.3)
      }
      try? "done \(Date().timeIntervalSince1970)".write(toFile: ack, atomically: true, encoding: .utf8)
    }
  }
}
