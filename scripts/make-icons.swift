// Draws the images the app ships, from the icon artwork and SF Symbols:
//
//   swift scripts/make-icons.swift
//
// - resources/icon.png, the app icon: the full-bleed artwork masked to the macOS icon shape (an
//   824-point rounded square on a 1024 canvas) with a soft shadow. `mygo build` makes the .icns from
//   it as it is, so the shape is drawn here.
// - assets/tray.png, the menu bar icon, a template image macOS tints for light and dark menu bars.
// - assets/sessions/*.png, the pre-market, post-market and overnight marks, white on transparent: the
//   panel draws them tinted with the color of the text beside them.
//
// Rerun it after changing the artwork; the outputs are committed.
import AppKit

let root = URL(fileURLWithPath: CommandLine.arguments[0]).deletingLastPathComponent().deletingLastPathComponent()

func bitmap(_ pixels: Int, draw: (NSGraphicsContext) -> Void) -> Data {
  let rep = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4,
    hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
  let context = NSGraphicsContext(bitmapImageRep: rep)!
  NSGraphicsContext.saveGraphicsState()
  NSGraphicsContext.current = context
  context.imageInterpolation = .high
  draw(context)
  NSGraphicsContext.restoreGraphicsState()
  return rep.representation(using: .png, properties: [:])!
}

func write(_ data: Data, _ path: String) {
  let url = root.appending(path: path)
  try! FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
  try! data.write(to: url)
  print("Wrote \(path)")
}

// The app icon.
guard let artwork = NSImage(contentsOf: root.appending(path: "assets/icon-source.png")) else {
  FileHandle.standardError.write(Data("Cannot read assets/icon-source.png\n".utf8))
  exit(1)
}
write(
  bitmap(1024) { _ in
    let body = NSRect(x: 100, y: 100, width: 824, height: 824)
    let shape = NSBezierPath(roundedRect: body, xRadius: 185, yRadius: 185)

    NSGraphicsContext.saveGraphicsState()
    let shadow = NSShadow()
    shadow.shadowColor = NSColor.black.withAlphaComponent(0.3)
    shadow.shadowOffset = NSSize(width: 0, height: -12)
    shadow.shadowBlurRadius = 24
    shadow.set()
    NSColor.black.setFill()
    shape.fill()
    NSGraphicsContext.restoreGraphicsState()

    NSGraphicsContext.saveGraphicsState()
    shape.addClip()
    artwork.draw(in: body, from: .zero, operation: .sourceOver, fraction: 1)
    NSGraphicsContext.restoreGraphicsState()
  }, "resources/icon.png")

/// An SF Symbol drawn in one color on transparent, centered in a square of `pixels`.
func symbol(_ name: String, pointSize: CGFloat, pixels: Int, color: NSColor = .black) -> Data {
  let config = NSImage.SymbolConfiguration(pointSize: pointSize, weight: .regular)
  guard let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)?.withSymbolConfiguration(config)
  else {
    FileHandle.standardError.write(Data("No SF Symbol named \(name)\n".utf8))
    exit(1)
  }
  return bitmap(pixels) { context in
    let scale = CGFloat(pixels) / 18
    context.cgContext.scaleBy(x: scale, y: scale)
    let size = image.size
    let rect = NSRect(x: (18 - size.width) / 2, y: (18 - size.height) / 2, width: size.width, height: size.height)
    image.draw(in: rect)
    color.set()
    rect.fill(using: .sourceAtop)
  }
}

// 18 points, the height of menu bar icons, at 2x.
write(symbol("chart.line.uptrend.xyaxis", pointSize: 15, pixels: 36), "assets/tray.png")

// A full sun rather than a sunrise, which at label size is indistinguishable from a sunset.
for (session, name) in [("pre", "sun.max.fill"), ("post", "sunset.fill"), ("overnight", "moon.stars.fill")] {
  write(symbol(name, pointSize: 14, pixels: 54, color: .white), "assets/sessions/\(session).png")
}
