export interface Entity {
  id: string;
  name: string;
  data: Record<string, string>;
}

export interface Repository {
  findById(id: string): Promise<Entity | null>;
  save(entity: Entity): Promise<void>;
}

export class Service {
  constructor(private readonly repo: Repository) {}

  async getEntity(id: string): Promise<Entity | null> {
    return this.repo.findById(id);
  }
}
