#pragma once
#include <string>
#include <memory>
#include <optional>

class Entity {
public:
    Entity(std::string id, std::string name) : id_(std::move(id)), name_(std::move(name)) {}
    const std::string& id() const { return id_; }
    const std::string& name() const { return name_; }
private:
    std::string id_;
    std::string name_;
};

class Repository {
public:
    virtual ~Repository() = default;
    virtual std::optional<Entity> find_by_id(const std::string& id) = 0;
    virtual void save(const Entity& entity) = 0;
};
