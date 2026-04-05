const std = @import("std");
const domain = @import("domain/entity.zig");

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();

    var repo = domain.Repository.init(allocator);
    const entity = domain.Entity.init("1", "test");
    try repo.save(entity);

    if (repo.findById("1")) |found| {
        std.debug.print("Found: {s}\n", .{found.name});
    }
}
