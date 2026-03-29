package domain;

import java.util.Map;

public class Entity {
    private final String id;
    private final String name;
    private final Map<String, String> data;

    public Entity(String id, String name, Map<String, String> data) {
        this.id = id;
        this.name = name;
        this.data = data;
    }

    public String getId() { return id; }
    public String getName() { return name; }
    public Map<String, String> getData() { return data; }
}
