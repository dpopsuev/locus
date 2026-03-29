import { z } from "zod";
import { Entity, Repository } from "../domain/entity";

const EntitySchema = z.object({
  id: z.string(),
  name: z.string(),
  data: z.record(z.string()),
});

export class ApiRepository implements Repository {
  constructor(private readonly baseUrl: string) {}

  async findById(id: string): Promise<Entity | null> {
    const resp = await fetch(`${this.baseUrl}/entities/${id}`);
    if (!resp.ok) return null;
    return EntitySchema.parse(await resp.json());
  }

  async save(entity: Entity): Promise<void> {
    await fetch(`${this.baseUrl}/entities`, {
      method: "POST",
      body: JSON.stringify(entity),
    });
  }
}
