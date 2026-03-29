"""HTTP adapter using requests library."""
import requests

from domain.entity import Entity, Repository


class HttpRepository(Repository):
    """Adapter implementing Repository via HTTP calls."""
    def __init__(self, base_url: str):
        self._base_url = base_url

    def find_by_id(self, id: str):
        resp = requests.get(f"{self._base_url}/entities/{id}")
        data = resp.json()
        return Entity(id=data["id"], name=data["name"], data=data)

    def save(self, entity: Entity):
        requests.post(f"{self._base_url}/entities", json={"id": entity.id, "name": entity.name})
