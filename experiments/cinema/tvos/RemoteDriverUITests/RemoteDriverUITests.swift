import XCTest

/// Development aid, not shipped: drives the Siri Remote in the tvOS Simulator so focus and
/// navigation can be walked from a script. It polls a command file and presses remote buttons.
///
/// Commands, one per line: up|down|left|right|select|menu|playpause|home [count], wait <seconds>,
/// hold (long-press select), launch. Each processed batch is acknowledged in `<file>.ack`.
final class RemoteDriverUITests: XCTestCase {
  func testRemoteDriver() throws {
    let env = ProcessInfo.processInfo.environment
    let path = env["REMOTE_DRIVER_FILE"] ?? "/tmp/cinema-remote/cmd.txt"
    let ack = path + ".ack"
    let app = XCUIApplication(bundleIdentifier: "tv.getportico.experiment.cinema.tv")
    let remote = XCUIRemote.shared
    var seen = ""
    let deadline = Date().addingTimeInterval(TimeInterval(env["REMOTE_DRIVER_MINUTES"].flatMap(Double.init) ?? 240) * 60)
    while Date() < deadline {
      guard let text = try? String(contentsOfFile: path, encoding: .utf8), text != seen, !text.isEmpty else {
        Thread.sleep(forTimeInterval: 0.25)
        continue
      }
      seen = text
      for raw in text.split(separator: "\n") {
        let parts = raw.split(separator: " ").map(String.init)
        guard let cmd = parts.first else { continue }
        let n = parts.count > 1 ? Int(parts[1]) ?? 1 : 1
        func press(_ b: XCUIRemote.Button) { for _ in 0..<n { remote.press(b); Thread.sleep(forTimeInterval: 0.35) } }
        switch cmd {
        case "up": press(.up)
        case "down": press(.down)
        case "left": press(.left)
        case "right": press(.right)
        case "select": press(.select)
        case "menu": press(.menu)
        case "playpause": press(.playPause)
        case "home": press(.home)
        case "hold": remote.press(.select, forDuration: 1.2)
        case "launch": app.activate()
        case "wait": Thread.sleep(forTimeInterval: Double(parts.count > 1 ? parts[1] : "1") ?? 1)
        case "stop": return
        default: break
        }
      }
      try? "done \(Date().timeIntervalSince1970)".write(toFile: ack, atomically: true, encoding: .utf8)
    }
  }
}
