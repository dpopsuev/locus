const std = @import("std");

const Entity = struct {
    id: []const u8,
    name: []const u8,
};

const Repository = struct {
    findByIdFn: *const fn (id: []const u8) ?Entity,
    saveFn: *const fn (entity: Entity) void,

    pub fn findById(self: Repository, id: []const u8) ?Entity {
        return self.findByIdFn(id);
    }

    pub fn save(self: Repository, entity: Entity) void {
        self.saveFn(entity);
    }
};

pub fn main() !void {
    const stdout = std.io.getStdOut().writer();
    try stdout.print("testkit-zig\n", .{});
}
