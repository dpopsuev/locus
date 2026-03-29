#!/usr/bin/env bash
# Entity library — sourced by main.sh

entity_new() {
    local id="$1"
    local name="$2"
    echo "${id}:${name}"
}

entity_get_id() {
    echo "${1%%:*}"
}

entity_get_name() {
    echo "${1#*:}"
}
