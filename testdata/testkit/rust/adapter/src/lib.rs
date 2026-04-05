use std::collections::HashMap;
use domain::{Entity, Repository};

pub struct MemoryRepo {
    data: HashMap<String, Entity>,
}

impl MemoryRepo {
    pub fn new() -> Self {
        Self {
            data: HashMap::new(),
        }
    }
}

impl Repository for MemoryRepo {
    fn find_by_id(&self, id: &str) -> Option<Entity> {
        self.data.get(id).cloned()
    }

    fn save(&self, _entity: &Entity) -> Result<(), String> {
        Ok(())
    }
}
