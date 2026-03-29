namespace TestKit.Domain;

public record Entity(string Id, string Name, Dictionary<string, string> Data);

public interface IRepository
{
    Task<Entity?> FindByIdAsync(string id);
    Task SaveAsync(Entity entity);
}
