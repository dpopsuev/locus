package domain;

import java.util.Optional;

public interface Repository {
    Optional<Entity> findById(String id);
    void save(Entity entity);
}
