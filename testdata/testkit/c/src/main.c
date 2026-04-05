#include <stdio.h>
#include <pthread.h>
#include "../include/entity.h"

int main(void) {
    Entity* e = entity_new("1", "test");
    if (e) {
        printf("Entity: %s (%s)\n", e->name, e->id);
        entity_free(e);
    }
    return 0;
}
