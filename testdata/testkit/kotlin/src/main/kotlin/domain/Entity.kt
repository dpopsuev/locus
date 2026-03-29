package domain

data class Entity(val id: String, val name: String, val data: Map<String, String> = emptyMap())

interface Repository {
    suspend fun findById(id: String): Entity?
    suspend fun save(entity: Entity)
}

sealed class Result<out T> {
    data class Success<T>(val value: T) : Result<T>()
    data class Failure(val error: String) : Result<Nothing>()
}
