using Microsoft.Extensions.Logging;
using TestKit.Domain;

namespace TestKit.Adapter;

public class InMemoryRepo : IRepository
{
    private readonly Dictionary<string, Entity> _store = new();
    private readonly ILogger<InMemoryRepo> _logger;

    public InMemoryRepo(ILogger<InMemoryRepo> logger) => _logger = logger;

    public Task<Entity?> FindByIdAsync(string id)
    {
        _logger.LogDebug("Finding {Id}", id);
        _store.TryGetValue(id, out var entity);
        return Task.FromResult(entity);
    }

    public Task SaveAsync(Entity entity)
    {
        _logger.LogInformation("Saving {Id}", entity.Id);
        _store[entity.Id] = entity;
        return Task.CompletedTask;
    }
}
