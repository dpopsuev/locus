class Entity {
  constructor(id, name, data = {}) {
    this.id = id;
    this.name = name;
    this.data = data;
  }
}

class Service {
  constructor(repository) {
    this.repo = repository;
  }

  getEntity(id) {
    return this.repo.findById(id);
  }
}

module.exports = { Entity, Service };
