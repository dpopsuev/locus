public struct Entity {
    public let id: String
    public let name: String
    public var data: [String: String]
}

public protocol Repository {
    func findById(_ id: String) async throws -> Entity?
    func save(_ entity: Entity) async throws
}
