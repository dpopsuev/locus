"""Core domain entities."""
from dataclasses import dataclass
from typing import Optional, Protocol


@dataclass
class Entity:
    id: str
    name: str
    data: dict


class Repository(Protocol):
    """Port interface for data access."""
    def find_by_id(self, id: str) -> Optional[Entity]: ...
    def save(self, entity: Entity) -> None: ...


class Service:
    """Domain service orchestrating business logic."""
    def __init__(self, repo: Repository):
        self._repo = repo

    def get_entity(self, id: str) -> Optional[Entity]:
        return self._repo.find_by_id(id)
