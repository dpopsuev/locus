#ifndef ENTITY_H
#define ENTITY_H

typedef struct {
    char id[64];
    char name[128];
} Entity;

typedef struct {
    Entity* (*find_by_id)(const char* id);
    int (*save)(const Entity* entity);
} Repository;

Entity* entity_new(const char* id, const char* name);
void entity_free(Entity* e);

#endif
