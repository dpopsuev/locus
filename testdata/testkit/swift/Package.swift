// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "testkit-swift",
    dependencies: [
        .package(url: "https://github.com/apple/swift-log.git", from: "1.5.0"),
    ],
    targets: [
        .target(name: "Domain", path: "Sources/Domain"),
        .target(name: "Adapter", dependencies: ["Domain", .product(name: "Logging", package: "swift-log")], path: "Sources/Adapter"),
    ]
)
