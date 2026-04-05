const std = @import("std");

pub const Entity = struct {
    id: []const u8,
    name: []const u8,

    pub fn init(id: []const u8, name: []const u8) Entity {
        return .{ .id = id, .name = name };
    }

    pub fn format(self: Entity, allocator: std.mem.Allocator) ![]u8 {
        return std.fmt.allocPrint(allocator, "{s}: {s}", .{ self.id, self.name });
    }
};

pub const Repository = struct {
    entities: std.StringHashMap(Entity),

    pub fn init(allocator: std.mem.Allocator) Repository {
        return .{ .entities = std.StringHashMap(Entity).init(allocator) };
    }

    pub fn save(self: *Repository, entity: Entity) !void {
        try self.entities.put(entity.id, entity);
    }

    pub fn findById(self: *Repository, id: []const u8) ?Entity {
        return self.entities.get(id);
    }
};
