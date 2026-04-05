use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Entity {
    pub id: String,
    pub name: String,
    pub data: HashMap<String, String>,
}

pub trait Repository {
    fn find_by_id(&self, id: &str) -> Option<Entity>;
    fn save(&self, entity: &Entity) -> Result<(), String>;
}

pub struct Service<R: Repository> {
    repo: R,
}

impl<R: Repository> Service<R> {
    pub fn new(repo: R) -> Self {
        Self { repo }
    }

    pub fn get_entity(&self, id: &str) -> Option<Entity> {
        self.repo.find_by_id(id)
    }
}
