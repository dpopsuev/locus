local socket = require("socket")

local Entity = {}
Entity.__index = Entity

function Entity.new(id, name)
    return setmetatable({ id = id, name = name, data = {} }, Entity)
end

function Entity:toString()
    return string.format("Entity(%s, %s)", self.id, self.name)
end

-- Repository via metatable (Strategy pattern)
local Repository = {}
Repository.__index = Repository

function Repository.new()
    return setmetatable({ store = {} }, Repository)
end

function Repository:findById(id)
    return self.store[id]
end

function Repository:save(entity)
    self.store[entity.id] = entity
end

return { Entity = Entity, Repository = Repository }
