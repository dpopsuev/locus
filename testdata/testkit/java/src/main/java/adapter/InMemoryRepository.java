package adapter;

import domain.Entity;
import domain.Repository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.HashMap;
import java.util.Map;
import java.util.Optional;

public class InMemoryRepository implements Repository {
    private static final Logger log = LoggerFactory.getLogger(InMemoryRepository.class);
    private final Map<String, Entity> store = new HashMap<>();

    @Override
    public Optional<Entity> findById(String id) {
        log.debug("Finding entity {}", id);
        return Optional.ofNullable(store.get(id));
    }

    @Override
    public void save(Entity entity) {
        log.info("Saving entity {}", entity.getId());
        store.put(entity.getId(), entity);
    }
}
