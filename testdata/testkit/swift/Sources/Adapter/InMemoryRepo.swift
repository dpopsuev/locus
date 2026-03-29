import Domain
import Logging

public class InMemoryRepo: Repository {
    private var store: [String: Entity] = [:]
    private let logger = Logger(label: "adapter")

    public func findById(_ id: String) async throws -> Entity? {
        logger.info("Finding \(id)")
        return store[id]
    }

    public func save(_ entity: Entity) async throws {
        logger.info("Saving \(entity.id)")
        store[entity.id] = entity
    }
}
