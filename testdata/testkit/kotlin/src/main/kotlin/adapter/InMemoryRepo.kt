package adapter

import domain.Entity
import domain.Repository
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

class InMemoryRepo : Repository {
    private val store = mutableMapOf<String, Entity>()
    private val mutex = Mutex()

    override suspend fun findById(id: String): Entity? = mutex.withLock { store[id] }
    override suspend fun save(entity: Entity) = mutex.withLock { store[entity.id] = entity }
}
