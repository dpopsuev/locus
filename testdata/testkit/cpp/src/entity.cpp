#include "../include/entity.hpp"
#include <unordered_map>

class InMemoryRepo : public Repository {
    std::unordered_map<std::string, Entity> store_;
public:
    std::optional<Entity> find_by_id(const std::string& id) override {
        auto it = store_.find(id);
        if (it == store_.end()) return std::nullopt;
        return it->second;
    }
    void save(const Entity& entity) override {
        store_.emplace(entity.id(), entity);
    }
};
