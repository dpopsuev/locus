#include <stdlib.h>
#include <string.h>
#include "../include/entity.h"

Entity* entity_new(const char* id, const char* name) {
    Entity* e = malloc(sizeof(Entity));
    if (!e) return NULL;
    strncpy(e->id, id, sizeof(e->id) - 1);
    strncpy(e->name, name, sizeof(e->name) - 1);
    return e;
}

void entity_free(Entity* e) { free(e); }
